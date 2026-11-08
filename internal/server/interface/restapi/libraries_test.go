package restapi_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestLibraries_DraftReleaseDiscard walks a library through the API: create,
// a WidgetType in its Draft, a Save, a Release of that Commit, the refusal of
// a second Release of it, the releases read back, a rename and a Discard.
func TestLibraries_DraftReleaseDiscard(t *testing.T) {
	router := newServices().router()
	const p = apiv0.PathPrefix
	name := "lib-" + uuid.NewString()

	rec := serve(router, http.MethodPost, p+"/libraries", apiv0.CreateLibraryRequest{Name: name})
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
	if rec := serve(router, http.MethodPost, lp+"/revert-to-release", nil); rec.Code != http.StatusConflict {
		t.Errorf("a Revert of a library never released: %d, want 409", rec.Code)
	}
	rec = serve(router, http.MethodPost, lp+"/widget-types", apiv0.CreateWidgetTypeRequest{
		Version: 1, Name: "gauge", HtmlTemplate: "<div/>", ScriptLanguage: "javascript", DefaultWidth: 10, DefaultHeight: 10,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST widget-types: %d %s", rec.Code, rec.Body)
	}
	wt := decodeBody[apiv0.CreateWidgetTypeResponse](t, rec)

	// A Release names a Commit, not the Draft.
	rec = serve(router, http.MethodPost, lp+"/commits", apiv0.CreateCommitRequest{Version: wt.Version, Title: "First", Author: "Ada"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST commits: %d %s", rec.Code, rec.Body)
	}
	commit := decodeBody[apiv0.CommitResponse](t, rec)

	rec = serve(router, http.MethodPost, lp+"/releases", apiv0.CreateReleaseRequest{Commit: commit.Number, Note: "first", Author: "Ada"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST releases: %d %s", rec.Code, rec.Body)
	}
	release := decodeBody[apiv0.LibraryReleaseResponse](t, rec)
	if release.Number != 1 || release.Note != "first" || release.CommitNumber == nil || *release.CommitNumber != commit.Number ||
		release.Title == nil || *release.Title != "First" || release.Author != "Ada" || release.Origin != apiv0.LibraryReleaseOriginCommit ||
		len(release.WidgetTypes) != 1 || release.WidgetTypes[0].ID != wt.ID || release.WidgetTypes[0].LibraryID != created.ID {
		t.Errorf("release: %+v", release)
	}
	if rec := serve(router, http.MethodPost, lp+"/releases", apiv0.CreateReleaseRequest{Commit: commit.Number, Author: "Ada"}); rec.Code != http.StatusConflict {
		t.Errorf("a released commit: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodPost, lp+"/releases", apiv0.CreateReleaseRequest{Commit: 9, Author: "Ada"}); rec.Code != http.StatusNotFound {
		t.Errorf("a missing commit: %d, want 404", rec.Code)
	}
	rec = serve(router, http.MethodPatch, lp+"/releases/1", apiv0.UpdateLibraryReleaseNoteRequest{Note: "renoted"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH release note: %d %s", rec.Code, rec.Body)
	}
	if updated := decodeBody[apiv0.LibraryReleaseResponse](t, rec); updated.Note != "renoted" || len(updated.WidgetTypes) != 1 {
		t.Errorf("the renoted release: %+v", updated)
	}
	if rec := serve(router, http.MethodGet, lp+"/releases/1", nil); rec.Code != http.StatusOK {
		t.Errorf("GET release 1: %d", rec.Code)
	}
	if rec := serve(router, http.MethodGet, lp+"/releases/2", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET release 2: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodGet, lp+"/releases/two", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("GET release two: %d, want 400", rec.Code)
	}
	if releases := decodeBody[apiv0.GetLibraryReleasesResponse](t, serve(router, http.MethodGet, lp+"/releases", nil)); len(releases.Releases) != 1 {
		t.Errorf("GET releases: %+v", releases)
	}

	rec = serve(router, http.MethodPut, lp, apiv0.UpdateLibraryRequest{Name: name + "-renamed", Version: created.Version})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT library: %d %s", rec.Code, rec.Body)
	}
	found := decodeBody[apiv0.GetLibrariesResponse](t, serve(router, http.MethodGet, p+"/libraries?name="+name+"-renamed", nil))
	if len(found.Libraries) != 1 || found.Libraries[0].LatestRelease != 1 || found.Libraries[0].LatestCommit != commit.Number ||
		found.Libraries[0].HasUnsavedChanges || found.Libraries[0].HasUnpublishedChanges {
		t.Errorf("GET ?name=: %+v", found)
	}

	rec = serve(router, http.MethodPost, lp+"/discard", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST discard: %d %s", rec.Code, rec.Body)
	}
	state := decodeBody[apiv0.DiscardLibraryResponse](t, rec)
	if state.Version <= wt.Version {
		t.Errorf("the Draft after the Discard: %+v, want a version newer than %d", state, wt.Version)
	}
	if list := decodeBody[apiv0.GetWidgetTypesResponse](t, serve(router, http.MethodGet, lp+"/widget-types", nil)); len(list.WidgetTypes) != 1 || list.WidgetTypes[0].ID != wt.ID {
		t.Errorf("the Draft after the Discard: %+v", list)
	}
	rec = serve(router, http.MethodPost, lp+"/revert-to-release", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST revert-to-release: %d %s", rec.Code, rec.Body)
	}
	if list := decodeBody[apiv0.GetWidgetTypesResponse](t, serve(router, http.MethodGet, lp+"/widget-types", nil)); len(list.WidgetTypes) != 1 || list.WidgetTypes[0].ID != wt.ID {
		t.Errorf("the Draft after the Revert: %+v", list)
	}
}

// TestLibraryPins_AddPreviewSwitchRemove: a pin is added, its preview names
// the Widgets a switch would orphan, and the switch is refused naming them.
func TestLibraryPins_AddPreviewSwitchRemove(t *testing.T) {
	cleanWidgetsRest(t)
	cleanWidgetTypes(t)
	router := newServices().router()
	lp := apiv0.PathPrefix + apiv0.LibraryPath(wtLibrary.String())
	pins := mainPath + "/library-pins/" + wtLibrary.String()

	wt := createWidgetTypeViaService(t, testWtInput)
	if rec := serve(router, http.MethodPut, pins, apiv0.PutLibraryPinRequest{Release: 1}); rec.Code != http.StatusBadRequest {
		t.Errorf("pinning a release that does not exist: %d, want 400", rec.Code)
	}
	commit := createLibraryCommitViaService(t, router, wtLibrary, wt.Version)
	serve(router, http.MethodPost, lp+"/releases", apiv0.CreateReleaseRequest{Commit: commit.Number, Author: "Ada"})
	rec := serve(router, http.MethodPut, pins, apiv0.PutLibraryPinRequest{Release: 1})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add pin: %d %s", rec.Code, rec.Body)
	}
	added := decodeBody[apiv0.LibraryPinResponse](t, rec)
	if added.CommitNumber == nil || *added.CommitNumber != commit.Number || added.Title == nil || *added.Title != "Save" ||
		added.Author != "Ada" || added.Origin != apiv0.LibraryReleaseOriginCommit {
		t.Errorf("the provenance of the pin: %+v, want commit 1 titled Save by Ada", added)
	}

	palette := decodeBody[apiv0.GetPinnedWidgetTypesResponse](t, serve(router, http.MethodGet, mainPath+"/widget-types", nil))
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
	second := createLibraryCommitViaService(t, router, wtLibrary, testLibraryDraftVersion(t, wtLibrary))
	serve(router, http.MethodPost, lp+"/releases", apiv0.CreateReleaseRequest{Commit: second.Number, Author: "Ada"})

	impact := decodeBody[apiv0.PinImpactResponse](t, serve(router, http.MethodGet, pins+"/impact?release=2", nil))
	if len(impact.OrphanedWidgets) != 1 || impact.OrphanedWidgets[0].WidgetID != w.ID {
		t.Errorf("impact: %+v", impact)
	}
	if rec := serve(router, http.MethodGet, pins+"/impact?release=x", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("impact of release x: %d, want 400", rec.Code)
	}

	rec = serve(router, http.MethodPut, pins, apiv0.PutLibraryPinRequest{Release: 2, Version: testDraftVersion(t, mainProject)})
	if rec.Code != http.StatusConflict {
		t.Fatalf("an orphaning switch: %d, want 409", rec.Code)
	}
	problem := decodeBody[apiv0.Problem](t, rec)
	if len(problem.OrphanedWidgets) != 1 || problem.OrphanedWidgets[0].WidgetID != w.ID || problem.OrphanedWidgets[0].SceneID != sc.ID {
		t.Errorf("the refusal names %+v, want the widget", problem.OrphanedWidgets)
	}
	if rec := serve(router, http.MethodDelete, pins, nil); rec.Code != http.StatusConflict {
		t.Errorf("an orphaning removal: %d, want 409", rec.Code)
	}

	serve(router, http.MethodDelete, mainPath+"/scenes/"+sc.ID.String()+"/widgets/"+w.ID.String(), nil)
	rec = serve(router, http.MethodPut, pins, apiv0.PutLibraryPinRequest{Release: 2, Version: testDraftVersion(t, mainProject)})
	if rec.Code != http.StatusOK {
		t.Fatalf("switch: %d %s", rec.Code, rec.Body)
	}
	switched := decodeBody[apiv0.LibraryPinResponse](t, rec)
	if switched.Release != 2 || switched.CommitNumber == nil || *switched.CommitNumber != second.Number {
		t.Errorf("the provenance of the switched pin: %+v", switched)
	}
	if list := decodeBody[apiv0.GetLibraryPinsResponse](t, serve(router, http.MethodGet, mainPath+"/library-pins", nil)); !hasPin(list, wtLibrary, 2) {
		t.Errorf("pins: %+v", list)
	}
	if rec := serve(router, http.MethodDelete, pins, nil); rec.Code != http.StatusOK {
		t.Errorf("remove: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodDelete, pins, nil); rec.Code != http.StatusNotFound {
		t.Errorf("remove again: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodDelete, apiv0.PathPrefix+apiv0.LibraryPath(wtLibrary.String()), nil); rec.Code != http.StatusOK {
		t.Errorf("delete the unpinned library: %d %s", rec.Code, rec.Body)
	}
}

func hasPin(list apiv0.GetLibraryPinsResponse, libraryID uuid.UUID, release int) bool {
	for _, p := range list.LibraryPins {
		if p.LibraryID == libraryID && p.Release == release {
			return true
		}
	}
	return false
}

// createLibraryCommitViaService saves the library's Draft as a Commit through
// the API, as the UI does before a Release (task 72_5; ADR 0021).
func createLibraryCommitViaService(t *testing.T, router servingRouter, libraryID uuid.UUID, version int) apiv0.CommitResponse {
	t.Helper()
	path := apiv0.PathPrefix + apiv0.LibraryPath(libraryID.String()) + "/commits"
	rec := serve(router, http.MethodPost, path, apiv0.CreateCommitRequest{Version: version, Title: "Save", Author: "Ada"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST commits: %d %s", rec.Code, rec.Body)
	}
	return decodeBody[apiv0.CommitResponse](t, rec)
}
