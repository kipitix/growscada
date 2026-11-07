package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// servingRouter is anything an HTTP test hands requests to: the API router or
// a testRouter wrapping it.
type servingRouter interface {
	ServeMux() http.Handler
}

// testRouter is the API router that fills the Version of the Draft in a write
// request: the tests are about what the server does with a request, not about
// stating the Version in every body and URL. A request that states one keeps
// it, so stale-version tests still see their conflict.
type testRouter struct {
	APIRouter *restapi.APIRouter
}

func (r testRouter) ServeMux() http.Handler {
	inner := r.APIRouter.ServeMux()
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		inner.ServeHTTP(w, withDraftVersion(req))
	})
}

// plainRouter is the API over the test database, with no test wrapper: a
// request is what it states.
func plainRouter() servingRouter {
	return restapi.NewRouter(newServices().services(), eventbus.NewEventBus(), 100)
}

// callAs sends a request through router with body encoded as JSON (nil: no
// body). The key parameter is ignored: the API names no client (ADR 0022).
func callAs(t *testing.T, router servingRouter, method, path string, body any, key string) *httptest.ResponseRecorder {
	t.Helper()
	_ = key
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)
	return rec
}

// cleanDrafts removes every Draft row and its DraftChanges: the tests start
// from a fresh Draft.
func cleanDrafts(t *testing.T) {
	t.Helper()
	if _, err := testDB().ExecContext(context.Background(),
		"DELETE FROM engineering.draft_changes; DELETE FROM engineering.drafts"); err != nil {
		t.Fatalf("clean drafts: %v", err)
	}
}

// projectIDInPath is the project a request path names, uuid.Nil if none.
func projectIDInPath(path string) uuid.UUID {
	rest, ok := strings.CutPrefix(path, apiv0.PathPrefix+"/projects/")
	if !ok {
		return uuid.Nil
	}
	raw, _, _ := strings.Cut(rest, "/")
	projectID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return projectID
}

// libraryIDInPath is the library a request path names, uuid.Nil if none.
func libraryIDInPath(path string) uuid.UUID {
	rest, ok := strings.CutPrefix(path, apiv0.PathPrefix+"/libraries/")
	if !ok {
		return uuid.Nil
	}
	raw, _, _ := strings.Cut(rest, "/")
	libraryID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return libraryID
}

// withDraftVersion fills the Version of the Draft in a write request of the
// test router: the tests are about what the server does with a request, not
// about stating the Version in every body and URL. A request that states one
// keeps it, so stale-version tests still see their conflict.
func withDraftVersion(req *http.Request) *http.Request {
	projectID := projectIDInPath(req.URL.Path)
	if projectID == uuid.Nil {
		return req
	}
	path := req.URL.Path
	if strings.HasSuffix(path, "/deploy") || strings.HasSuffix(path, "/discard") || strings.Contains(path, "/deployments") ||
		strings.HasSuffix(path, "/value") || strings.Contains(path, "/token") ||
		strings.Contains(path, "/projectfile") {
		return req
	}
	version := draftVersionInDB(projectID)
	switch req.Method {
	case http.MethodDelete:
		if req.URL.Query().Get(apiv0.VersionQuery) == "" {
			query := req.URL.Query()
			query.Set(apiv0.VersionQuery, strconv.Itoa(version))
			req.URL.RawQuery = query.Encode()
		}
	case http.MethodPost, http.MethodPut:
		if req.Body == nil {
			return req
		}
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		var object map[string]any
		if err != nil || json.Unmarshal(body, &object) != nil || object == nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
			return req
		}
		if stated, _ := object["version"].(float64); stated == 0 {
			object["version"] = version
			body, _ = json.Marshal(object)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
	}
	return req
}

// draftVersionInDB reads the Version of a Project's Draft, 1 for a Draft
// whose row the migration did not make.
func draftVersionInDB(projectID uuid.UUID) int {
	var version int
	if err := testDB().QueryRow(`SELECT version FROM engineering.drafts WHERE project_id = $1`, projectID).Scan(&version); err != nil {
		return 1
	}
	return version
}
