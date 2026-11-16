package restapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestLoadSavedState_Project: a saved state becomes what is edited (task 84).
// The Draft is replaced by the saved state's content in one transaction and is
// clean afterwards, whichever saved state it loaded (ADR 0016, amendment
// 2026-11-16). A number the Project does not have is 404 and changes nothing.
func TestLoadSavedState_Project(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	router := services.router()

	if rec := serve(router, http.MethodPost, mainPath+"/saved-states/1/load", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("load without a SavedState: %d, want 404", rec.Code)
	}

	version := services.draftVersion(t)
	rec := serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Version: version, Name: "speed", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	version = services.draftVersion(t)
	if rec := serve(router, http.MethodPost, mainPath+"/saved-states",
		apiv0.CreateSavedStateRequest{Version: version, Title: "The first", Author: "Ada"}); rec.Code != http.StatusCreated {
		t.Fatalf("POST saved state: %d %s", rec.Code, rec.Body)
	}

	// A Tag the saved state does not hold: the Draft differs from it.
	version = services.draftVersion(t)
	if rec := serve(router, http.MethodPost, mainPath+"/tags",
		apiv0.CreateTagRequest{Version: version, Name: "pressure", Type: "integer"}); rec.Code != http.StatusCreated {
		t.Fatalf("POST second tag: %d %s", rec.Code, rec.Body)
	}

	rec = serve(router, http.MethodPost, mainPath+"/saved-states/1/load", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("load the saved state: %d %s", rec.Code, rec.Body)
	}
	loaded := decodeBody[apiv0.LoadSavedStateResponse](t, rec)
	if loaded.Version == 0 {
		t.Errorf("the load states no draft version: %+v", loaded)
	}
	if loaded.HasUnsavedChanges {
		t.Errorf("the last saved state loaded leaves the Draft dirty: %+v", loaded)
	}
	if len(loaded.Warnings) != 0 {
		t.Errorf("the load dropped a pin: %+v", loaded.Warnings)
	}
	rec = serve(router, http.MethodGet, mainPath+"/tags", nil)
	tags := decodeBody[apiv0.GetTagsResponse](t, rec).Tags
	if len(tags) != 1 || tags[0].Name != "speed" {
		t.Errorf("the Draft after the load: %+v", tags)
	}
	// The stack is cleared: a load is a reset, not a change to undo.
	if rec := serve(router, http.MethodGet, mainPath+"/draft-changes", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET draft-changes: %d %s", rec.Code, rec.Body)
	} else if state := decodeBody[apiv0.DraftChangesResponse](t, rec); state.CanUndo {
		t.Errorf("the load left the DraftChange stack: %+v", state)
	}

	// A second saved state makes 1 an older one: loading it leaves the Draft
	// clean too, which the rule of 2026-11-10 did not (ADR 0016, amendment
	// 2026-11-16).
	version = services.draftVersion(t)
	if rec := serve(router, http.MethodPost, mainPath+"/tags",
		apiv0.CreateTagRequest{Version: version, Name: "flow", Type: "integer"}); rec.Code != http.StatusCreated {
		t.Fatalf("POST third tag: %d %s", rec.Code, rec.Body)
	}
	version = services.draftVersion(t)
	if rec := serve(router, http.MethodPost, mainPath+"/saved-states",
		apiv0.CreateSavedStateRequest{Version: version, Title: "The second", Author: "Ada"}); rec.Code != http.StatusCreated {
		t.Fatalf("POST second saved state: %d %s", rec.Code, rec.Body)
	}
	rec = serve(router, http.MethodPost, mainPath+"/saved-states/1/load", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("load the older saved state: %d %s", rec.Code, rec.Body)
	}
	if older := decodeBody[apiv0.LoadSavedStateResponse](t, rec); older.HasUnsavedChanges {
		t.Errorf("an older saved state loaded leaves the Draft dirty: %+v", older)
	}

	rec = serve(router, http.MethodPost, mainPath+"/saved-states/99/load", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("load a saved state the Project does not have: %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("a refusal's content type: %q", got)
	}
}

// TestLoadSavedState_Library: the same reset of a Library's Draft, to the
// WidgetTypes its saved state copied (task 84).
func TestLoadSavedState_Library(t *testing.T) {
	router := newServices().router()
	const p = apiv0.PathPrefix

	rec := serve(router, http.MethodPost, p+"/libraries", apiv0.CreateLibraryRequest{Name: "load-" + uuid.NewString()})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /libraries: %d %s", rec.Code, rec.Body)
	}
	lp := p + apiv0.LibraryPath(decodeBody[apiv0.CreateLibraryResponse](t, rec).ID.String())

	rec = serve(router, http.MethodPost, lp+"/widget-types", apiv0.CreateWidgetTypeRequest{
		Version: 1, Name: "gauge", HtmlTemplate: "<div/>", ScriptLanguage: "javascript", DefaultWidth: 10, DefaultHeight: 10,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST widget-types: %d %s", rec.Code, rec.Body)
	}
	saved := decodeBody[apiv0.CreateWidgetTypeResponse](t, rec)
	if rec := serve(router, http.MethodPost, lp+"/saved-states",
		apiv0.CreateSavedStateRequest{Version: saved.Version, Title: "One widget", Author: "Ada"}); rec.Code != http.StatusCreated {
		t.Fatalf("POST saved states: %d %s", rec.Code, rec.Body)
	}

	// A second WidgetType, which the saved state does not hold.
	rec = serve(router, http.MethodPost, lp+"/widget-types", apiv0.CreateWidgetTypeRequest{
		Version: saved.Version, Name: "meter", HtmlTemplate: "<div/>", ScriptLanguage: "javascript", DefaultWidth: 10, DefaultHeight: 10,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST second widget-type: %d %s", rec.Code, rec.Body)
	}

	rec = serve(router, http.MethodPost, lp+"/saved-states/1/load", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("load the saved state: %d %s", rec.Code, rec.Body)
	}
	loaded := decodeBody[apiv0.LoadSavedStateResponse](t, rec)
	if loaded.Version == 0 || loaded.HasUnsavedChanges {
		t.Errorf("the load of the last SavedState: %+v", loaded)
	}
	types := decodeBody[apiv0.GetWidgetTypesResponse](t, serve(router, http.MethodGet, lp+"/widget-types", nil)).WidgetTypes
	if len(types) != 1 || types[0].Name != "gauge" {
		t.Errorf("the Draft after the load: %+v", types)
	}

	if rec := serve(router, http.MethodPost, lp+"/saved-states/9/load", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("load a saved state the library does not have: %d %s", rec.Code, rec.Body)
	}
}
