package restapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// The DraftChanges routes (ADR 0017): the state of a Draft's stack and its
// undo and redo, under a Project's path and a Library's.

// callRoutes sends a request through router with body encoded as JSON (nil:
// no body) and returns the response.
func callRoutes(t *testing.T, router servingRouter, method, path string, body any) *httptest.ResponseRecorder {
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
	rec := httptest.NewRecorder()
	router.ServeMux().ServeHTTP(rec, req)
	return rec
}

func decodeDraftChanges(t *testing.T, rec *httptest.ResponseRecorder) apiv0.DraftChangesResponse {
	t.Helper()
	var resp apiv0.DraftChangesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode draft changes: %v", err)
	}
	return resp
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) apiv0.Problem {
	t.Helper()
	var problem apiv0.Problem
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return problem
}

func TestGetDraftChanges_AfterAChange(t *testing.T) {
	cleanDrafts(t)
	cleanTags(t)
	router := newRouter()
	if rec := callRoutes(t, router, http.MethodPost, mainPath+"/tags",
		map[string]any{"name": "pressure", "type": "integer"}); rec.Code != http.StatusCreated {
		t.Fatalf("create tag: %d %s", rec.Code, rec.Body.String())
	}

	rec := callRoutes(t, router, http.MethodGet, mainPath+apiv0.DraftChangesPath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	state := decodeDraftChanges(t, rec)
	if state.Version != 2 || !state.CanUndo || state.CanRedo {
		t.Errorf("the state: %+v, want version 2 with an undo", state)
	}
	if state.UndoDescription != "Create tag pressure" || state.RedoDescription != "" {
		t.Errorf("the descriptions: undo %q, redo %q", state.UndoDescription, state.RedoDescription)
	}
}

func TestPostDraftChanges_UndoAndRedo(t *testing.T) {
	cleanDrafts(t)
	cleanTags(t)
	router := newRouter()
	if rec := callRoutes(t, router, http.MethodPost, mainPath+"/tags",
		map[string]any{"name": "pressure", "type": "integer"}); rec.Code != http.StatusCreated {
		t.Fatalf("create tag: %d %s", rec.Code, rec.Body.String())
	}

	rec := callRoutes(t, router, http.MethodPost, mainPath+apiv0.DraftChangesUndoPath, map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body.String())
	}
	undone := decodeDraftChanges(t, rec)
	if undone.Version != 3 || undone.CanUndo || !undone.CanRedo || undone.RedoDescription != "Create tag pressure" {
		t.Errorf("after the undo: %+v, want version 3 with a redo", undone)
	}
	if rec := callRoutes(t, router, http.MethodGet, mainPath+"/tags", nil); rec.Code == http.StatusOK {
		var list apiv0.GetTagsResponse
		if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
			t.Fatalf("decode tags: %v", err)
		}
		if len(list.Tags) != 0 {
			t.Errorf("the undone Tag is still listed: %+v", list.Tags)
		}
	} else {
		t.Fatalf("list tags: %d", rec.Code)
	}

	rec = callRoutes(t, router, http.MethodPost, mainPath+apiv0.DraftChangesRedoPath, map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("redo: %d %s", rec.Code, rec.Body.String())
	}
	redone := decodeDraftChanges(t, rec)
	if redone.Version != 4 || !redone.CanUndo || redone.CanRedo {
		t.Errorf("after the redo: %+v, want version 4 with an undo and nothing to redo", redone)
	}

	rec = callRoutes(t, router, http.MethodPost, mainPath+apiv0.DraftChangesRedoPath, map[string]any{})
	if rec.Code != http.StatusConflict {
		t.Fatalf("redo of an empty tail: %d, want 409", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content type %q, want problem+json", ct)
	}
	problem := decodeProblem(t, rec)
	if problem.Status != http.StatusConflict || problem.Detail == "" {
		t.Errorf("the problem: %+v", problem)
	}
}

func TestPostDraftChanges_EmptyStack(t *testing.T) {
	cleanDrafts(t)
	router := lockRouter()
	path := mainPath + apiv0.DraftChangesUndoPath

	rec := callAs(t, router, http.MethodPost, path, apiv0.DraftChangeCommandRequest{Version: 1}, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("undo of an empty stack: %d, want 409", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content type %q, want problem+json", ct)
	}
	problem := decodeProblem(t, rec)
	if problem.Status != http.StatusConflict || problem.Detail == "" {
		t.Errorf("the problem: %+v", problem)
	}
}

func TestGetDraftChanges_ProjectNotFound(t *testing.T) {
	router := newRouter()
	path := apiv0.PathPrefix + "/projects/" + uuid.NewString() + apiv0.DraftChangesPath
	if rec := callRoutes(t, router, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestLibraryDraftChanges_UndoAndRedo(t *testing.T) {
	cleanDrafts(t)
	cleanWidgetTypes(t)
	router := newRouter()
	created := createWidgetTypeViaService(t, testWtInput)
	path := apiv0.PathPrefix + apiv0.LibraryPath(wtLibrary.String())

	rec := callRoutes(t, router, http.MethodGet, path+apiv0.DraftChangesPath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("state: %d %s", rec.Code, rec.Body.String())
	}
	state := decodeDraftChanges(t, rec)
	if state.Version != created.Version || !state.CanUndo || state.CanRedo {
		t.Fatalf("the library's state: %+v, want version %d with an undo", state, created.Version)
	}

	rec = callRoutes(t, router, http.MethodPost, path+apiv0.DraftChangesUndoPath,
		apiv0.DraftChangeCommandRequest{Version: state.Version})
	if rec.Code != http.StatusOK {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body.String())
	}
	undone := decodeDraftChanges(t, rec)
	if !undone.CanRedo || undone.CanUndo {
		t.Errorf("after the undo: %+v, want a redo", undone)
	}
	rec = callRoutes(t, router, http.MethodGet, path+"/widget-types", nil)
	var list apiv0.GetWidgetTypesResponse
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode widget types: %v", err)
	}
	if len(list.WidgetTypes) != 0 {
		t.Errorf("the undone WidgetType is still listed: %+v", list.WidgetTypes)
	}

	rec = callRoutes(t, router, http.MethodPost, path+apiv0.DraftChangesRedoPath,
		apiv0.DraftChangeCommandRequest{Version: undone.Version})
	if rec.Code != http.StatusOK {
		t.Fatalf("redo: %d %s", rec.Code, rec.Body.String())
	}
	rec = callRoutes(t, router, http.MethodGet, path+"/widget-types", nil)
	list = apiv0.GetWidgetTypesResponse{}
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode widget types: %v", err)
	}
	if len(list.WidgetTypes) != 1 || list.WidgetTypes[0].Name != "gauge" {
		t.Errorf("the redone WidgetType: %+v, want gauge", list.WidgetTypes)
	}
}

func TestLibraryDraftChanges_NotFound(t *testing.T) {
	router := newRouter()
	path := apiv0.PathPrefix + apiv0.LibraryPath(uuid.NewString()) + apiv0.DraftChangesPath
	if rec := callRoutes(t, router, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestLibraryDraftChanges_EmptyStackNamesTheLibrary(t *testing.T) {
	cleanDrafts(t)
	cleanWidgetTypes(t)
	router := newRouter()
	path := apiv0.PathPrefix + apiv0.LibraryPath(wtLibrary.String()) + apiv0.DraftChangesUndoPath
	rec := callRoutes(t, router, http.MethodPost, path, apiv0.DraftChangeCommandRequest{Version: 1})
	if rec.Code != http.StatusConflict {
		t.Fatalf("undo of an empty stack: %d, want 409", rec.Code)
	}
	problem := decodeProblem(t, rec)
	if problem.Title != "Conflict: Library" {
		t.Errorf("title %q, want the Library's", problem.Title)
	}
}
