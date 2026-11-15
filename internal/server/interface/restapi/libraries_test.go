package restapi_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestLibraries_DraftSaveDiscard walks a library through the API: create, a
// WidgetType in its Draft, a Save, the history read back, a rename and a
// Discard (task 85_7).
func TestLibraries_DraftSaveDiscard(t *testing.T) {
	router := newServices().router()
	const p = apiv0.PathPrefix
	name := "lib-" + uuid.NewString()

	rec := serve(router, http.MethodPost, p+"/libraries", apiv0.CreateLibraryRequest{Name: name, Description: "  Shared widgets  "})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /libraries: %d %s", rec.Code, rec.Body)
	}
	created := decodeBody[apiv0.CreateLibraryResponse](t, rec)
	lp := p + apiv0.LibraryPath(created.ID.String())

	if rec := serve(router, http.MethodPost, p+"/libraries", apiv0.CreateLibraryRequest{Name: name}); rec.Code != http.StatusConflict {
		t.Errorf("a taken name: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodPost, lp+"/discard", nil); rec.Code != http.StatusConflict {
		t.Errorf("a Discard of a library never saved: %d, want 409", rec.Code)
	}
	rec = serve(router, http.MethodPost, lp+"/widget-types", apiv0.CreateWidgetTypeRequest{
		Version: 1, Name: "gauge", HtmlTemplate: "<div/>", ScriptLanguage: "javascript", DefaultWidth: 10, DefaultHeight: 10,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST widget-types: %d %s", rec.Code, rec.Body)
	}
	wt := decodeBody[apiv0.CreateWidgetTypeResponse](t, rec)

	// A Save fixes the Draft as its saved state.
	rec = serve(router, http.MethodPost, lp+"/saved-states", apiv0.CreateSavedStateRequest{Version: wt.Version, Title: "First", Author: "Ada"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST commits: %d %s", rec.Code, rec.Body)
	}
	commit := decodeBody[apiv0.SavedStateResponse](t, rec)
	if commit.Number != 1 || commit.Title != "First" || commit.Author != "Ada" || commit.ContentCounts.WidgetTypes != 1 {
		t.Errorf("saved state: %+v", commit)
	}
	if commits := decodeBody[apiv0.GetSavedStatesResponse](t, serve(router, http.MethodGet, lp+"/saved-states", nil)); len(commits.SavedStates) != 1 {
		t.Errorf("GET commits: %+v", commits)
	}

	rec = serve(router, http.MethodPut, lp, apiv0.UpdateLibraryRequest{Name: name + "-renamed", Description: ptr("Renamed"), Version: created.Version})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT library: %d %s", rec.Code, rec.Body)
	}
	found := decodeBody[apiv0.GetLibrariesResponse](t, serve(router, http.MethodGet, p+"/libraries?name="+name+"-renamed", nil))
	if len(found.Libraries) != 1 || found.Libraries[0].LatestSavedState != commit.Number || found.Libraries[0].HasUnsavedChanges {
		t.Errorf("GET ?name=: %+v", found)
	}
	if got := found.Libraries[0].Description; got != "Renamed" {
		t.Errorf("the description after a PUT: %q, want %q", got, "Renamed")
	}
	// An update without a description keeps the current one (task 73).
	if rec := serve(router, http.MethodPut, lp, apiv0.UpdateLibraryRequest{Name: name + "-renamed", Version: created.Version + 1}); rec.Code != http.StatusOK {
		t.Fatalf("PUT without a description: %d %s", rec.Code, rec.Body)
	}
	if got := decodeBody[apiv0.LibraryResponse](t, serve(router, http.MethodGet, lp, nil)); got.Description != "Renamed" {
		t.Errorf("a PUT without a description erased it: %q", got.Description)
	}

	// A write after the Save, then a Discard to the saved state.
	state := decodeBody[apiv0.DraftStateResponse](t, serve(router, http.MethodGet, lp+"/draft", nil))
	rec = serve(router, http.MethodPost, lp+"/widget-types", apiv0.CreateWidgetTypeRequest{
		Version: state.Version, Name: "dial", HtmlTemplate: "<div/>", ScriptLanguage: "javascript", DefaultWidth: 10, DefaultHeight: 10,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST widget-types: %d %s", rec.Code, rec.Body)
	}
	rec = serve(router, http.MethodPost, lp+"/discard", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST discard: %d %s", rec.Code, rec.Body)
	}
	discarded := decodeBody[apiv0.DiscardLibraryResponse](t, rec)
	if discarded.Version <= state.Version {
		t.Errorf("the Draft after the Discard: %+v, want a version newer than %d", discarded, state.Version)
	}
	if list := decodeBody[apiv0.GetWidgetTypesResponse](t, serve(router, http.MethodGet, lp+"/widget-types", nil)); len(list.WidgetTypes) != 1 || list.WidgetTypes[0].ID != wt.ID {
		t.Errorf("the Draft after the Discard: %+v", list)
	}
	// The removed routes answer 404: they are gone, not stubbed.
	for _, path := range []string{lp + "/releases", lp + "/releases/1", lp + "/revert-to-release"} {
		if rec := serve(router, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", path, rec.Code)
		}
	}
}

// TestLibraryImports_AddPreviewSwitchRemove: an import is added, its preview
// names the Widgets a switch would orphan, and the switch is refused naming
// them (task 85_6).
func TestLibraryImports_AddPreviewSwitchRemove(t *testing.T) {
	cleanWidgetsRest(t)
	cleanWidgetTypes(t)
	router := newServices().router()
	lp := apiv0.PathPrefix + apiv0.LibraryPath(wtLibrary.String())
	imports := mainPath + "/library-imports/" + wtLibrary.String()

	wt := createWidgetTypeViaService(t, testWtInput)
	if rec := serve(router, http.MethodPut, imports, apiv0.PutLibraryImportRequest{SavedState: 1}); rec.Code != http.StatusNotFound {
		t.Errorf("importing a saved state that does not exist: %d, want 404", rec.Code)
	}
	commit := createLibrarySavedStateViaService(t, router, wtLibrary, wt.Version)
	rec := serve(router, http.MethodPut, imports, apiv0.PutLibraryImportRequest{SavedState: commit.Number})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add import: %d %s", rec.Code, rec.Body)
	}
	added := decodeBody[apiv0.LibraryImportResponse](t, rec)
	if added.SavedStateNumber != commit.Number || added.Title != "Save" || added.Author != "Ada" {
		t.Errorf("the provenance of the import: %+v, want saved state 1 titled Save by Ada", added)
	}

	palette := decodeBody[apiv0.GetImportedWidgetTypesResponse](t, serve(router, http.MethodGet, mainPath+"/library-imports/widget-types", nil))
	found := false
	for _, pwt := range palette.WidgetTypes {
		found = found || pwt.ID == wt.ID
	}
	if !found {
		t.Errorf("the palette lacks the pinned type: %+v", palette)
	}

	sc := createSceneViaSceneService(t)
	input := testWidgetInput
	input.TypeID = wt.ID
	w := createWidgetViaService(t, sc.ID, sc.Version, input)

	serve(router, http.MethodDelete, lp+"/widget-types/"+wt.ID.String()+"?version="+strconv.Itoa(testLibraryDraftVersion(t, wtLibrary)), nil)
	second := createLibrarySavedStateViaService(t, router, wtLibrary, testLibraryDraftVersion(t, wtLibrary))

	impact := decodeBody[apiv0.ImportImpactResponse](t, serve(router, http.MethodGet, imports+"/impact?saved_state="+strconv.Itoa(second.Number), nil))
	if len(impact.OrphanedWidgets) != 1 || impact.OrphanedWidgets[0].WidgetID != w.ID {
		t.Errorf("impact: %+v", impact)
	}
	if rec := serve(router, http.MethodGet, imports+"/impact?saved_state=x", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("impact of saved state x: %d, want 400", rec.Code)
	}

	rec = serve(router, http.MethodPut, imports, apiv0.PutLibraryImportRequest{SavedState: second.Number, Version: testDraftVersion(t, mainProject)})
	if rec.Code != http.StatusConflict {
		t.Fatalf("an orphaning switch: %d, want 409", rec.Code)
	}
	problem := decodeBody[apiv0.Problem](t, rec)
	if len(problem.OrphanedWidgets) != 1 || problem.OrphanedWidgets[0].WidgetID != w.ID || problem.OrphanedWidgets[0].SceneID != sc.ID {
		t.Errorf("the refusal names %+v, want the widget", problem.OrphanedWidgets)
	}
	if rec := serve(router, http.MethodDelete, imports, nil); rec.Code != http.StatusConflict {
		t.Errorf("an orphaning removal: %d, want 409", rec.Code)
	}

	serve(router, http.MethodDelete, mainPath+"/scenes/"+sc.ID.String()+"/widgets/"+w.ID.String(), nil)
	rec = serve(router, http.MethodPut, imports, apiv0.PutLibraryImportRequest{SavedState: second.Number, Version: testDraftVersion(t, mainProject)})
	if rec.Code != http.StatusOK {
		t.Fatalf("switch: %d %s", rec.Code, rec.Body)
	}
	switched := decodeBody[apiv0.LibraryImportResponse](t, rec)
	if switched.SavedStateNumber != second.Number {
		t.Errorf("the provenance of the switched import: %+v", switched)
	}
	if list := decodeBody[apiv0.GetLibraryImportsResponse](t, serve(router, http.MethodGet, mainPath+"/library-imports", nil)); !hasImport(list, wtLibrary, second.Number) {
		t.Errorf("imports: %+v", list)
	}
	if rec := serve(router, http.MethodDelete, imports, nil); rec.Code != http.StatusOK {
		t.Errorf("remove: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodDelete, imports, nil); rec.Code != http.StatusNotFound {
		t.Errorf("remove again: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, apiv0.PathPrefix+apiv0.LibraryPath(wtLibrary.String()), nil); rec.Code != http.StatusOK {
		t.Errorf("delete the unpinned library: %d %s", rec.Code, rec.Body)
	}
}

func hasImport(list apiv0.GetLibraryImportsResponse, libraryID uuid.UUID, commit int) bool {
	for _, imported := range list.LibraryImports {
		if imported.LibraryID == libraryID && imported.SavedStateNumber == commit {
			return true
		}
	}
	return false
}

// createLibrarySavedStateViaService saves the library's Draft as a saved state through
// the API, as the UI does before an Import (task 85_6).
func createLibrarySavedStateViaService(t *testing.T, router servingRouter, libraryID uuid.UUID, version int) apiv0.SavedStateResponse {
	t.Helper()
	path := apiv0.PathPrefix + apiv0.LibraryPath(libraryID.String()) + "/saved-states"
	rec := serve(router, http.MethodPost, path, apiv0.CreateSavedStateRequest{Version: version, Title: "Save", Author: "Ada"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST commits: %d %s", rec.Code, rec.Body)
	}
	return decodeBody[apiv0.SavedStateResponse](t, rec)
}
