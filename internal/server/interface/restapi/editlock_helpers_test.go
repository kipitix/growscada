package restapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// testClientKey is the client the tests' HTTP requests act as. The EditLock of
// a project under test holds it (holdEditLockFor): a test writes the Draft
// through the API, which needs the lock (ADR 0015). A test of the lock itself
// drives the routes with its own keys through restapi.NewRouter directly.
var testClientKey = uuid.MustParse("d0000000-0000-4000-8000-00000000d00d")

// servingRouter is anything an HTTP test hands requests to: the API router or
// a testRouter wrapping it.
type servingRouter interface {
	ServeMux() http.Handler
}

// testRouter is the API router with testClientKey on every request, holding
// the EditLock of every project the request names.
type testRouter struct {
	APIRouter *restapi.APIRouter
}

func (r testRouter) ServeMux() http.Handler {
	inner := r.APIRouter.ServeMux()
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.Header.Set(apiv0.ClientHeader, testClientKey.String())
		if projectID := projectIDInPath(req.URL.Path); projectID != uuid.Nil {
			holdEditLockFor(projectID)
		}
		if libraryID := libraryIDInPath(req.URL.Path); libraryID != uuid.Nil {
			holdLibraryEditLockFor(libraryID)
		}
		inner.ServeHTTP(w, withDraftVersion(req))
	})
}

// holdTestEditLock makes testClientKey the holder of the Main project's
// EditLock, whatever it was.
func holdTestEditLock() { holdEditLockFor(mainProject) }

// holdEditLockFor makes testClientKey the holder of the EditLock of the
// project projectID. A project that does not exist gets no row: the request
// is answered 404 before any write.
func holdEditLockFor(projectID uuid.UUID) {
	db := testDB()
	_, _ = db.Exec(`INSERT INTO engineering.drafts (id, project_id, lock_version) VALUES (gen_random_uuid(), $1, 1) ON CONFLICT DO NOTHING`, projectID)
	_, _ = db.Exec(
		`UPDATE engineering.drafts SET holder_key = $2, holder_name = 'restapi-test', held_since = now(), last_seen = now()
		 WHERE project_id = $1`, projectID, testClientKey)
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

// holdLibraryEditLockFor makes testClientKey the holder of the EditLock of the
// Draft of the library libraryID. A library that does not exist gets no row:
// the request is answered 404 before any write.
func holdLibraryEditLockFor(libraryID uuid.UUID) {
	db := testDB()
	_, _ = db.Exec(`INSERT INTO engineering.drafts (id, library_id, lock_version) VALUES (gen_random_uuid(), $1, 1) ON CONFLICT DO NOTHING`, libraryID)
	_, _ = db.Exec(
		`UPDATE engineering.drafts SET holder_key = $2, holder_name = 'restapi-test', held_since = now(), last_seen = now()
		 WHERE library_id = $1`, libraryID, testClientKey)
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
	if strings.HasSuffix(path, "/deploy") || strings.HasSuffix(path, "/discard") || strings.Contains(path, "/revisions") ||
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
