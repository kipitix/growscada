package restapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestLibraries_DraftReleaseDiscard walks a library through the API: create,
// a WidgetType in its Draft, a Release, the refusal of an unchanged one, the
// releases read back, a rename and a Discard.
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
		t.Errorf("a Discard of a library never released: %d, want 409", rec.Code)
	}
	rec = serve(router, http.MethodPost, lp+"/widget-types", apiv0.CreateWidgetTypeRequest{
		Name: "gauge", HtmlTemplate: "<div/>", ScriptLanguage: "javascript", DefaultWidth: 10, DefaultHeight: 10,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST widget-types: %d %s", rec.Code, rec.Body)
	}
	wt := decodeBody[apiv0.CreateWidgetTypeResponse](t, rec)

	rec = serve(router, http.MethodPost, lp+"/releases", apiv0.CreateReleaseRequest{Note: "first", Version: created.Version})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST releases: %d %s", rec.Code, rec.Body)
	}
	release := decodeBody[apiv0.LibraryReleaseResponse](t, rec)
	if release.Number != 1 || release.Note != "first" || len(release.WidgetTypes) != 1 || release.WidgetTypes[0].ID != wt.ID || release.WidgetTypes[0].LibraryID != created.ID {
		t.Errorf("release: %+v", release)
	}
	if rec := serve(router, http.MethodPost, lp+"/releases", apiv0.CreateReleaseRequest{Version: created.Version}); rec.Code != http.StatusConflict {
		t.Errorf("an unchanged Draft: %d, want 409", rec.Code)
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
	if len(found.Libraries) != 1 || found.Libraries[0].LatestRelease != 1 || found.Libraries[0].HasUnreleasedChanges {
		t.Errorf("GET ?name=: %+v", found)
	}

	rec = serve(router, http.MethodPost, lp+"/discard", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST discard: %d %s", rec.Code, rec.Body)
	}
	if lib := decodeBody[apiv0.LibraryResponse](t, rec); lib.DiscardingTo != 1 {
		t.Errorf("the library being discarded: %+v", lib)
	}
	if rec := serve(router, http.MethodDelete, lp+"/widget-types/"+wt.ID.String(), nil); rec.Code != http.StatusConflict {
		t.Errorf("an edit during the Discard: %d, want 409", rec.Code)
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
	serve(router, http.MethodPost, lp+"/releases", apiv0.CreateReleaseRequest{Version: 1})
	rec := serve(router, http.MethodPut, pins, apiv0.PutLibraryPinRequest{Release: 1})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add pin: %d %s", rec.Code, rec.Body)
	}
	pin := decodeBody[apiv0.LibraryPinResponse](t, rec)

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

	serve(router, http.MethodDelete, lp+"/widget-types/"+wt.ID.String(), nil)
	serve(router, http.MethodPost, lp+"/releases", apiv0.CreateReleaseRequest{Version: 1})

	impact := decodeBody[apiv0.PinImpactResponse](t, serve(router, http.MethodGet, pins+"/impact?release=2", nil))
	if len(impact.OrphanedWidgets) != 1 || impact.OrphanedWidgets[0].WidgetID != w.ID {
		t.Errorf("impact: %+v", impact)
	}
	if rec := serve(router, http.MethodGet, pins+"/impact?release=x", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("impact of release x: %d, want 400", rec.Code)
	}

	rec = serve(router, http.MethodPut, pins, apiv0.PutLibraryPinRequest{Release: 2, Version: pin.Version})
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
	if rec := serve(router, http.MethodPut, pins, apiv0.PutLibraryPinRequest{Release: 2, Version: pin.Version}); rec.Code != http.StatusOK {
		t.Fatalf("switch: %d %s", rec.Code, rec.Body)
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

