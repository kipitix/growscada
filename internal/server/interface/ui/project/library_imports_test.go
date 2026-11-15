package project

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// libraryWithState creates a library of its own with a WidgetType named
// "gauge" and saves its Draft as the library's first saved state.
func libraryWithState(t *testing.T, api *testAPI) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	lib, err := api.CreateLibrary(ctx, apiv0.CreateLibraryRequest{Name: "lib-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	created, err := api.Library(lib.ID).CreateWidgetType(ctx, apiv0.CreateWidgetTypeRequest{
		Name: "gauge", HtmlTemplate: "<div></div>", ScriptLanguage: "javascript",
		DefaultWidth: 150, DefaultHeight: 70, InputPorts: []apiv0.InputPort{},
	})
	if err != nil {
		t.Fatalf("create widget type: %v", err)
	}
	if _, err := api.Library(lib.ID).SaveSavedState(ctx, apiv0.CreateSavedStateRequest{
		Version: created.Version, Title: "First save", Author: "Ada"}); err != nil {
		t.Fatalf("save saved state: %v", err)
	}
	return lib.ID, created.ID
}

// saveLibraryState saves the library's Draft as its next saved state after
// touching it: a Save of a Draft nothing was written to is a 409.
func saveLibraryState(t *testing.T, api *testAPI, libraryID uuid.UUID, title string) apiv0.SavedStateResponse {
	t.Helper()
	ctx := context.Background()
	state, err := api.Library(libraryID).DraftState(ctx)
	if err != nil {
		t.Fatalf("draft state: %v", err)
	}
	saved, err := api.Library(libraryID).SaveSavedState(ctx, apiv0.CreateSavedStateRequest{
		Version: state.Version, Title: title, Author: "Ada"})
	if err != nil {
		t.Fatalf("save saved state: %v", err)
	}
	return saved
}

// importOf asks the server what the project imports of the library.
func importOf(t *testing.T, api *testAPI, libraryID uuid.UUID) (apiv0.LibraryImportResponse, bool) {
	t.Helper()
	imports, err := api.Imports(context.Background())
	if err != nil {
		t.Fatalf("imports: %v", err)
	}
	for _, imported := range imports {
		if imported.LibraryID == libraryID {
			return imported, true
		}
	}
	return apiv0.LibraryImportResponse{}, false
}

// TestImportWindow_ListsTheLibrariesAndTheirSavedStates: the window shows every
// Library with what the project imports of it, and the saved states of the one
// opened, newest first, each with its title, Author and number of WidgetTypes
// (task 85_6).
func TestImportWindow_ListsTheLibrariesAndTheirSavedStates(t *testing.T) {
	f := startProject(t)
	f.run()
	libraryID, _ := libraryWithState(t, f.api)

	f.p.openImportDialog(f.p.compoCtx)
	f.run()
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	for _, want := range []string{
		`data-import-dialog`, "Import libraries",
		`data-import-library-row="` + libraryID.String() + `"`,
		`data-import-state="` + libraryID.String() + `"`,
		"Not imported",
		"latest saved state #1",
		`data-import-filter`, "All labels",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the window does not show %q: %s", want, html)
		}
	}

	f.p.selectImportLibrary(f.p.compoCtx, libraryID)
	f.run()
	f.noToasts()
	html = app.HTMLString(f.p.Render())
	for _, want := range []string{
		`data-import-saved-states="` + libraryID.String() + `"`,
		`data-import-saved-state="1"`,
		"First save", "Ada", "1 widget types",
		`data-import-saved-state-action="1"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the opened library does not show %q: %s", want, html)
		}
	}

	f.p.closeImportDialog()
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, "No libraries imported yet.") || !strings.Contains(html, "Import libraries…") {
		t.Errorf("the panel does not offer the import: %s", html)
	}
}

// TestImportWindow_NoLibrariesSaysSo: a server without libraries says how the
// list would fill, rather than showing an empty window (task 85_6).
func TestImportWindow_NoLibrariesSaysSo(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.openImportDialog(f.p.compoCtx)
	f.run()
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-import-no-libraries`) || !strings.Contains(html, "No libraries yet.") {
		t.Errorf("the empty window does not say there are no libraries: %s", html)
	}
}

// TestImportWindow_LibraryWithoutSavedStatesSaysHowToMakeOne: a Library that
// was never saved has nothing to import, and the window says so (task 85_6).
func TestImportWindow_LibraryWithoutSavedStatesSaysHowToMakeOne(t *testing.T) {
	f := startProject(t)
	f.run()
	lib, err := f.api.CreateLibrary(context.Background(), apiv0.CreateLibraryRequest{Name: "lib-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}

	f.p.openImportDialog(f.p.compoCtx)
	f.run()
	f.p.selectImportLibrary(f.p.compoCtx, lib.ID)
	f.run()
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-import-no-saved-states`) ||
		!strings.Contains(html, "No saved states: save the library first.") {
		t.Errorf("the library without saved states does not say how to make one: %s", html)
	}
}

// TestImportWindow_ImportSwitchAndRemove: the row of a saved state imports it,
// the row of another switches the import and its own names the removal, and
// every answer rereads the panel, the palette and the Draft's state
// (task 85_6).
func TestImportWindow_ImportSwitchAndRemove(t *testing.T) {
	f := startProject(t)
	f.run()
	libraryID, typeID := libraryWithState(t, f.api)

	f.p.openImportDialog(f.p.compoCtx)
	f.run()
	f.p.selectImportLibrary(f.p.compoCtx, libraryID)
	f.run()
	f.p.changeImport(libraryID, 1)
	f.run()
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-import-provenance="1"`) || !strings.Contains(html, "saved state #1 First save · Ada") {
		t.Errorf("the panel does not show the import: %s", html)
	}
	if !strings.Contains(html, `data-import-current="1"`) {
		t.Errorf("the imported saved state is not marked as imported: %s", html)
	}
	if _, ok := importOf(t, f.api, libraryID); !ok {
		t.Fatal("the server does not hold the import")
	}
	if !hasWidgetType(f.p.widgetTypes, typeID) {
		t.Errorf("the palette does not hold the imported WidgetType: %+v", f.p.widgetTypes)
	}

	// Another saved state, with the types the library's Draft holds then.
	if _, err := f.api.Library(libraryID).UpdateWidgetType(context.Background(), typeID, apiv0.UpdateWidgetTypeRequest{
		Name: "gauge", HtmlTemplate: "<p></p>", ScriptLanguage: "javascript",
		DefaultWidth: 150, DefaultHeight: 70, InputPorts: []apiv0.InputPort{},
	}); err != nil {
		t.Fatalf("update widget type: %v", err)
	}
	saveLibraryState(t, f.api, libraryID, "Second save")
	f.p.selectImportLibrary(f.p.compoCtx, libraryID)
	f.run()
	f.p.changeImport(libraryID, 2)
	f.run()
	f.noToasts()
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-import-provenance="2"`) || !strings.Contains(html, "saved state #2 Second save · Ada") {
		t.Errorf("the switched import is not shown: %s", html)
	}
	if imported, ok := importOf(t, f.api, libraryID); !ok || imported.SavedStateNumber != 2 {
		t.Errorf("the server's import: %+v, %t; want saved state 2", imported, ok)
	}

	f.p.changeImport(libraryID, 0)
	f.run()
	f.noToasts()
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-import-none`) {
		t.Errorf("the panel still shows an import: %s", html)
	}
	if _, ok := importOf(t, f.api, libraryID); ok {
		t.Error("the server still holds the import")
	}
	if hasWidgetType(f.p.widgetTypes, typeID) {
		t.Errorf("the palette still holds the removed library's WidgetType: %+v", f.p.widgetTypes)
	}
}

// TestImportWindow_OrphaningSwitchIsRefusedNamingTheWidgets: a switch that
// would leave a Widget without its WidgetType writes nothing, and the toast
// names the Widget (task 85_6, ADR 0012).
func TestImportWindow_OrphaningSwitchIsRefusedNamingTheWidgets(t *testing.T) {
	f := startProject(t)
	f.run()
	libraryID, typeID := libraryWithState(t, f.api)
	if _, err := f.api.ImportLibrary(context.Background(), libraryID, apiv0.PutLibraryImportRequest{SavedState: 1}); err != nil {
		t.Fatalf("import: %v", err)
	}
	sceneID := createScene(t, f.api, "Main")
	createWidget(t, f.api, sceneID, typeID, "gauge-1")

	// The next saved state no longer holds the WidgetType.
	ctx := context.Background()
	if _, err := f.api.Library(libraryID).DeleteWidgetType(ctx, typeID, 0); err != nil {
		t.Fatalf("delete widget type: %v", err)
	}
	saveLibraryState(t, f.api, libraryID, "Without the gauge")

	f.p.openImportDialog(f.p.compoCtx)
	f.run()
	f.p.selectImportLibrary(f.p.compoCtx, libraryID)
	f.run()
	f.p.changeImport(libraryID, 2)
	f.run()
	shown := f.oneToast()
	if !strings.Contains(shown.Title, "Cannot import saved state 2 of") {
		t.Errorf("the toast's title: %q", shown.Title)
	}
	if !strings.Contains(shown.Detail, "gauge-1") {
		t.Errorf("the toast does not name the Widget: %q", shown.Detail)
	}
	if imported, ok := importOf(t, f.api, libraryID); !ok || imported.SavedStateNumber != 1 {
		t.Errorf("the refused switch changed the import: %+v, %t", imported, ok)
	}
}

// TestImportWindow_FilterByLabel: the filter is the server's — GET
// …/saved-states?label=<id> —, All labels asks for every saved state, and the list
// of the libraries is never narrowed by it (task 85_3, task 85_6).
func TestImportWindow_FilterByLabel(t *testing.T) {
	url, api := newServer(t)
	log := &uriLog{}
	f := mountWith(t, url, api, &http.Client{Transport: log})
	f.run()
	libraryID, typeID := libraryWithState(t, api)
	if _, err := api.Library(libraryID).UpdateWidgetType(context.Background(), typeID, apiv0.UpdateWidgetTypeRequest{
		Name: "gauge", HtmlTemplate: "<p></p>", ScriptLanguage: "javascript",
		DefaultWidth: 150, DefaultHeight: 70, InputPorts: []apiv0.InputPort{},
	}); err != nil {
		t.Fatalf("update widget type: %v", err)
	}
	saveLibraryState(t, api, libraryID, "Second save")
	label, err := api.CreateLabel(context.Background(), "import-"+uuid.NewString()[:8], "#22c55e")
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	if _, err := api.Library(libraryID).AssignLabel(context.Background(), 2, label.ID); err != nil {
		t.Fatalf("assign label: %v", err)
	}

	f.p.openImportDialog(f.p.compoCtx)
	f.run()
	f.p.selectImportLibrary(f.p.compoCtx, libraryID)
	f.run()
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-import-saved-state="1"`) || !strings.Contains(html, `data-import-saved-state="2"`) {
		t.Errorf("All labels does not list every saved state: %s", html)
	}

	log.take()
	f.p.setImportLabel(f.p.compoCtx, label.ID)
	f.run()
	f.noToasts()
	html = app.HTMLString(f.p.Render())
	if strings.Contains(html, `data-import-saved-state="1"`) {
		t.Errorf("the label filter kept a saved state it does not mark: %s", html)
	}
	if !strings.Contains(html, `data-import-saved-state="2"`) {
		t.Errorf("the label filter dropped the saved state it marks: %s", html)
	}
	if !strings.Contains(html, `data-import-library-row="`+libraryID.String()+`"`) {
		t.Errorf("the label filter narrowed the list of libraries: %s", html)
	}
	asked := false
	for _, req := range log.take() {
		if strings.Contains(req, apiv0.SavedStatePath+"?"+apiv0.SavedStatesQueryLabel+"="+label.ID.String()) {
			asked = true
		}
	}
	if !asked {
		t.Errorf("the filter is not the server's: no GET with ?label=%s", label.ID)
	}

	f.p.setImportLabel(f.p.compoCtx, uuid.Nil)
	f.run()
	f.noToasts()
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-import-saved-state="1"`) || !strings.Contains(html, `data-import-saved-state="2"`) {
		t.Errorf("All labels does not bring every saved state back: %s", html)
	}
}

// TestImportWindow_AnotherClientsImportRereadsThePanel: an import another tab
// makes reaches the panel and the palette through library_import_created
// (task 85_6).
func TestImportWindow_AnotherClientsImportRereadsThePanel(t *testing.T) {
	f := startProject(t)
	f.run()
	libraryID, typeID := libraryWithState(t, f.api)
	if _, err := f.api.ImportLibrary(context.Background(), libraryID, apiv0.PutLibraryImportRequest{SavedState: 1}); err != nil {
		t.Fatalf("import: %v", err)
	}

	// The event of the Draft's write in the other tab.
	f.event(apiv0.Event{Type: apiv0.EventLibraryImportCreated, ID: libraryID.String()})
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-import-row="`+libraryID.String()+`"`) ||
		!strings.Contains(html, `data-import-provenance="1"`) {
		t.Errorf("the panel did not follow the other tab's import: %s", html)
	}
	if !hasWidgetType(f.p.widgetTypes, typeID) {
		t.Errorf("the palette did not follow the other tab's import: %+v", f.p.widgetTypes)
	}
}

// hasWidgetType reports whether the palette holds the WidgetType typeID.
func hasWidgetType(palette []apiv0.WidgetTypeCopyResponse, typeID uuid.UUID) bool {
	for _, wt := range palette {
		if wt.ID == typeID {
			return true
		}
	}
	return false
}
