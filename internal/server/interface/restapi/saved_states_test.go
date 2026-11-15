package restapi_test

import (
	"context"
	"net/http"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// TestSavedStates_ProjectSaveListFindRename covers the saved state routes of a
// Project (task 72_2; ADR 0021): a Save, its refusals, the history, one
// saved state, the rename and the ProjectFile download.
func TestSavedStates_ProjectSaveListFindRename(t *testing.T) {
	cleanProjectsRest(t)
	if _, err := testDB().ExecContext(context.Background(),
		"DELETE FROM engineering.drafts"); err != nil {
		t.Fatalf("clean drafts: %v", err)
	}
	services := newServices()
	router := services.router()

	if rec := serve(router, http.MethodGet, mainPath+"/saved-states", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET saved states: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/saved-states", apiv0.CreateSavedStateRequest{
		Version: 1, Title: "  First save ", Author: " Ada ",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("POST saved state: %d %s", rec.Code, rec.Body)
	}
	created := decodeBody[apiv0.SavedStateResponse](t, serve(router, http.MethodGet, mainPath+"/saved-states/1", nil))
	if created.Number != 1 || created.Title != "First save" || created.Author != "Ada" {
		t.Errorf("the saved state: %+v", created)
	}

	if rec := serve(router, http.MethodPost, mainPath+"/saved-states", apiv0.CreateSavedStateRequest{
		Version: 1, Title: "Again", Author: "Ada",
	}); rec.Code != http.StatusConflict {
		t.Errorf("a Save without changes: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/saved-states", apiv0.CreateSavedStateRequest{
		Version: 1, Title: "", Author: "Ada",
	}); rec.Code != http.StatusBadRequest {
		t.Errorf("a Save without a title: %d, want 400", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/saved-states", apiv0.CreateSavedStateRequest{
		Version: 0, Title: "Stale", Author: "Ada",
	}); rec.Code != http.StatusConflict {
		t.Errorf("a Save at a stale version: %d, want 409", rec.Code)
	}

	rec := serve(router, http.MethodGet, mainPath+"/saved-states", nil)
	list := decodeBody[apiv0.GetSavedStatesResponse](t, rec)
	if len(list.SavedStates) != 1 || list.SavedStates[0].Number != 1 {
		t.Errorf("history: %+v", list)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/saved-states/9", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a missing saved state: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/saved-states/not-a-number", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("GET a saved state by a non-number: %d, want 400", rec.Code)
	}

	rec = serve(router, http.MethodPatch, mainPath+"/saved-states/1", apiv0.RenameSavedStateRequest{Title: "Before the demo"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH saved state: %d %s", rec.Code, rec.Body)
	}
	if renamed := decodeBody[apiv0.SavedStateResponse](t, rec); renamed.Title != "Before the demo" {
		t.Errorf("renamed saved state: %+v", renamed)
	}
	if rec := serve(router, http.MethodPatch, mainPath+"/saved-states/1", apiv0.RenameSavedStateRequest{Title: " "}); rec.Code != http.StatusBadRequest {
		t.Errorf("PATCH with an empty title: %d, want 400", rec.Code)
	}

	rec = serve(router, http.MethodGet, mainPath+"/saved-states/1/projectfile", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET the saved state's project file: %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("project file content type %q", ct)
	}
}

// TestSavedStates_LibrarySaveAndRead covers the saved state routes of a Library
// (task 72_2; ADR 0021).
func TestSavedStates_LibrarySaveAndRead(t *testing.T) {
	services := newServices()
	router := services.router()
	created, err := services.libraries.CreateLibrary(context.Background(),
		appdto.LibraryInput{Name: "saved-states-rest-" + t.Name()})
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	path := apiv0.PathPrefix + apiv0.LibraryPath(created.ID.String())

	if rec := serve(router, http.MethodGet, path+"/saved-states", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET library saved states: %d %s", rec.Code, rec.Body)
	}
	if _, err := services.widgetTypes.CreateWidgetType(context.Background(), created.ID, 1, appdto.WidgetTypeInput{
		Name: "gauge", HtmlTemplate: "<div></div>", Script: "function update(){}",
		ScriptLanguage: "javascript", DefaultWidth: 100, DefaultHeight: 100,
	}); err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	if rec := serve(router, http.MethodPost, path+"/saved-states", apiv0.CreateSavedStateRequest{
		Version: 2, Title: "Widgets", Author: "Ada",
	}); rec.Code != http.StatusCreated {
		t.Fatalf("POST library saved state: %d %s", rec.Code, rec.Body)
	}
	rec := serve(router, http.MethodGet, path+"/saved-states/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET library saved state: %d %s", rec.Code, rec.Body)
	}
	if saved := decodeBody[apiv0.SavedStateResponse](t, rec); saved.WidgetTypes == nil {
		t.Errorf("a library's saved state carries no widget types: %+v", saved)
	}
	if rec := serve(router, http.MethodPatch, path+"/saved-states/1", apiv0.RenameSavedStateRequest{Title: "For the demo"}); rec.Code != http.StatusOK {
		t.Errorf("PATCH library saved state: %d, want 200", rec.Code)
	}
	if rec := serve(router, http.MethodGet, path+"/saved-states/1/projectfile", nil); rec.Code != http.StatusNotFound {
		t.Errorf("a library has no saved state project file: %d, want 404", rec.Code)
	}
}
