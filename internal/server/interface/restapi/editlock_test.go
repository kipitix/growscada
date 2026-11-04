package restapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
	"github.com/kipitix/growscada/internal/server/interface/restapi"
)

// The EditLock routes (ADR 0015). The routers here are raw: the test client
// header is what each request states, so the refusals are those of the
// server, not of the test wrapper.

func cleanEditLocksRest(t *testing.T) {
	t.Helper()
	if _, err := testDB().ExecContext(editlock.System(context.Background()), "DELETE FROM engineering.edit_locks"); err != nil {
		t.Fatalf("clean edit locks: %v", err)
	}
}

// lockRouter is the API over the test database with no test client: each
// request states its own.
func lockRouter() servingRouter {
	return restapi.NewRouter(newServices().services(), eventbus.NewEventBus(), 100)
}

// callAs sends a request naming the client key ("" for none).
func callAs(t *testing.T, router servingRouter, method, path string, body any, key string) *httptest.ResponseRecorder {
	t.Helper()
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
	if key != "" {
		req.Header.Set(apiv0.ClientHeader, key)
	}
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)
	return rec
}

func decodeLock(t *testing.T, rec *httptest.ResponseRecorder) apiv0.EditLockResponse {
	t.Helper()
	var resp apiv0.EditLockResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode lock: %v", err)
	}
	return resp
}

func lockPath() string { return mainPath + apiv0.EditLockPath }

var (
	aliceKey = uuid.MustParse("a1000000-0000-4000-8000-000000000001")
	bobKey   = uuid.MustParse("b1000000-0000-4000-8000-000000000002")
)

func TestGetEditLock_Free_Is200(t *testing.T) {
	cleanEditLocksRest(t)
	rec := callAs(t, lockRouter(), http.MethodGet, lockPath(), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	lock := decodeLock(t, rec)
	if lock.Held || lock.Holder != nil || lock.Version != 0 {
		t.Errorf("free lock: %+v", lock)
	}
}

func TestPostEditLock_WithoutClient_Is400(t *testing.T) {
	cleanEditLocksRest(t)
	rec := callAs(t, lockRouter(), http.MethodPost, lockPath(), apiv0.CheckOutEditLockRequest{Name: "alice"}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestPostEditLock_Free_Is201Then200(t *testing.T) {
	cleanEditLocksRest(t)
	router := lockRouter()

	rec := callAs(t, router, http.MethodPost, lockPath(), apiv0.CheckOutEditLockRequest{Name: "alice"}, aliceKey.String())
	if rec.Code != http.StatusCreated {
		t.Fatalf("check out: %d %s", rec.Code, rec.Body)
	}
	lock := decodeLock(t, rec)
	if !lock.Held || lock.Holder == nil || lock.Holder.Name != "alice" || lock.Holder.Key != aliceKey || lock.Version != 1 {
		t.Fatalf("checked out: %+v", lock)
	}

	rec = callAs(t, router, http.MethodPost, lockPath(), apiv0.CheckOutEditLockRequest{Name: "alice"}, aliceKey.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("check out again: %d, want 200", rec.Code)
	}
	if again := decodeLock(t, rec); again.Version != lock.Version {
		t.Errorf("check out again raised the version to %d", again.Version)
	}
}

func TestPostEditLock_HeldByAnother_Is409WithTheHolder(t *testing.T) {
	cleanEditLocksRest(t)
	router := lockRouter()
	callAs(t, router, http.MethodPost, lockPath(), apiv0.CheckOutEditLockRequest{Name: "alice"}, aliceKey.String())

	rec := callAs(t, router, http.MethodPost, lockPath(), apiv0.CheckOutEditLockRequest{Name: "bob"}, bobKey.String())
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", rec.Code)
	}
	var problem apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.EditLock == nil || !problem.EditLock.Held || problem.EditLock.Holder == nil || problem.EditLock.Holder.Name != "alice" {
		t.Errorf("the problem does not name the holder: %+v", problem.EditLock)
	}
}

func TestDeleteEditLock_CheckIn(t *testing.T) {
	cleanEditLocksRest(t)
	router := lockRouter()
	callAs(t, router, http.MethodPost, lockPath(), apiv0.CheckOutEditLockRequest{Name: "alice"}, aliceKey.String())

	if rec := callAs(t, router, http.MethodDelete, lockPath(), nil, bobKey.String()); rec.Code != http.StatusConflict {
		t.Errorf("check in of another's lock: %d, want 409", rec.Code)
	}
	rec := callAs(t, router, http.MethodDelete, lockPath(), nil, aliceKey.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("check in: %d", rec.Code)
	}
	if lock := decodeLock(t, rec); lock.Held || lock.Version != 2 {
		t.Errorf("checked in: %+v", lock)
	}
	if rec := callAs(t, router, http.MethodDelete, lockPath(), nil, aliceKey.String()); rec.Code != http.StatusOK {
		t.Errorf("check in of a free lock: %d, want 200", rec.Code)
	}
}

func TestPostEditLockTakeOver(t *testing.T) {
	cleanEditLocksRest(t)
	router := lockRouter()
	callAs(t, router, http.MethodPost, lockPath(), apiv0.CheckOutEditLockRequest{Name: "alice"}, aliceKey.String())

	if rec := callAs(t, router, http.MethodPost, lockPath()+"/takeover",
		apiv0.TakeOverEditLockRequest{Name: "bob", Version: 7}, bobKey.String()); rec.Code != http.StatusConflict {
		t.Errorf("takeover with a stale version: %d, want 409", rec.Code)
	}
	rec := callAs(t, router, http.MethodPost, lockPath()+"/takeover",
		apiv0.TakeOverEditLockRequest{Name: "bob", Version: 1}, bobKey.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("takeover: %d %s", rec.Code, rec.Body)
	}
	lock := decodeLock(t, rec)
	if lock.Holder == nil || lock.Holder.Name != "bob" || lock.Version != 2 {
		t.Errorf("taken over: %+v", lock)
	}
}

func TestPostEditLockTakeOver_FreeLock_Is409(t *testing.T) {
	cleanEditLocksRest(t)
	rec := callAs(t, lockRouter(), http.MethodPost, lockPath()+"/takeover",
		apiv0.TakeOverEditLockRequest{Name: "bob", Version: 0}, bobKey.String())
	if rec.Code != http.StatusConflict {
		t.Errorf("takeover of a free lock: %d, want 409", rec.Code)
	}
}

func TestEditLock_UnknownProject_Is404(t *testing.T) {
	rec := callAs(t, lockRouter(), http.MethodGet,
		apiv0.PathPrefix+apiv0.ProjectPath(uuid.NewString())+apiv0.EditLockPath, nil, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}

func TestClientKey_Malformed_Is400(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, mainPath+"/tags", nil)
	req.Header.Set(apiv0.ClientHeader, "not-a-uuid")
	rec := httptest.NewRecorder()
	lockRouter().ServeMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
}

func TestGatedWrite_OverREST(t *testing.T) {
	cleanTags(t)
	cleanEditLocksRest(t)
	router := lockRouter()

	tag := apiv0.CreateTagRequest{Name: "gated", Type: "string"}
	if rec := callAs(t, router, http.MethodPost, mainPath+"/tags", tag, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("write without a client: %d, want 400", rec.Code)
	}
	if rec := callAs(t, router, http.MethodPost, mainPath+"/tags", tag, aliceKey.String()); rec.Code != http.StatusConflict {
		t.Errorf("write of a free Draft: %d, want 409", rec.Code)
	} else {
		var problem apiv0.Problem
		if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
			t.Fatalf("decode problem: %v", err)
		}
		if problem.EditLock == nil || problem.EditLock.Held || problem.EditLock.Holder != nil {
			t.Errorf("the problem of a free Draft: %+v", problem.EditLock)
		}
	}

	callAs(t, router, http.MethodPost, lockPath(), apiv0.CheckOutEditLockRequest{Name: "alice"}, aliceKey.String())
	if rec := callAs(t, router, http.MethodPost, mainPath+"/tags", tag, bobKey.String()); rec.Code != http.StatusConflict {
		t.Errorf("write of another client's Draft: %d, want 409", rec.Code)
	}
	if rec := callAs(t, router, http.MethodPost, mainPath+"/tags", tag, aliceKey.String()); rec.Code != http.StatusCreated {
		t.Errorf("the holder's write: %d %s", rec.Code, rec.Body)
	}
}

func TestCheckOut_WhileDiscarding_Is409(t *testing.T) {
	cleanEditLocksRest(t)
	router := lockRouter()
	// The seeded Main project has no Revision, so no Discard of it can start;
	// the mark Check out reads is set directly.
	markDiscarding(t)

	rec := callAs(t, router, http.MethodPost, mainPath+apiv0.EditLockPath,
		apiv0.CheckOutEditLockRequest{Name: "alice"}, aliceKey.String())
	if rec.Code != http.StatusConflict {
		t.Fatalf("check out while discarding: %d %s, want 409", rec.Code, rec.Body)
	}
	var problem apiv0.Problem
	_ = json.NewDecoder(rec.Body).Decode(&problem)
	if problem.Detail == "" {
		t.Errorf("no detail of the refusal")
	}
}

// markDiscarding marks the Main project as being discarded, as a Discard
// under way does, without needing a Revision.
func markDiscarding(t *testing.T) {
	t.Helper()
	if _, err := testDB().ExecContext(editlock.System(context.Background()),
		`UPDATE projects SET discarding = true WHERE id = $1`, mainProject); err != nil {
		t.Fatalf("mark discarding: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB().ExecContext(editlock.System(context.Background()),
			`UPDATE projects SET discarding = false WHERE id = $1`, mainProject)
	})
}
