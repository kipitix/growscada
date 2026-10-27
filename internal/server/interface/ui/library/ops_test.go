package library

import (
	"context"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uitest"
	"github.com/kipitix/growscada/internal/server/servertest"
)

// The operations of Library run against the real REST API over Postgres.

var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// mountOnHost lets go-app mount a Library on the host (see uitest).
type mountOnHost struct{ *Library }

func (m mountOnHost) OnPreRender(ctx app.Context) { m.OnMount(ctx) }

// fixture is a mounted Library and a second client of the same server, which
// arranges and checks the server's state as another user would.
type fixture struct {
	t   *testing.T
	l   *Library
	e   *uitest.Engine
	api *testAPI
}

func newServer(t *testing.T) (string, *testAPI) {
	t.Helper()
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	return srv.URL, newTestAPI(t, apiclient.New(srv.URL, srv.Client()))
}

func mount(t *testing.T, url string, api *testAPI) *fixture {
	t.Helper()
	l := NewLibrary(apiclient.New(url, http.DefaultClient))
	f := &fixture{t: t, l: l, api: api}
	f.e = uitest.Load(t, mountOnHost{l})
	return f
}

func (f *fixture) run() { f.e.ConsumeAll() }

func (f *fixture) noToasts() {
	f.t.Helper()
	if toasts := f.e.TakeToasts(); len(toasts) > 0 {
		f.t.Fatalf("unexpected toasts: %+v", toasts)
	}
}

func (f *fixture) oneToast() toast.Problem {
	f.t.Helper()
	toasts := f.e.TakeToasts()
	if len(toasts) != 1 {
		f.t.Fatalf("want one toast, got %+v", toasts)
	}
	return toasts[0]
}

func (f *fixture) widgetType(id uuid.UUID) apiv0.WidgetTypeResponse {
	f.t.Helper()
	wt, err := f.api.GetWidgetType(context.Background(), id)
	if err != nil {
		f.t.Fatalf("get widget type: %v", err)
	}
	return wt
}

func createWidgetType(t *testing.T, api *testAPI, name string) uuid.UUID {
	t.Helper()
	created, err := api.CreateWidgetType(context.Background(), apiv0.CreateWidgetTypeRequest{
		Name: name, HtmlTemplate: "<div></div>", Script: "function render(inputs) {}", ScriptLanguage: "javascript",
		DefaultWidth: 150, DefaultHeight: 70, InputPorts: []apiv0.InputPort{{Name: "value", TypeHint: "integer"}},
	})
	if err != nil {
		t.Fatalf("create widget type: %v", err)
	}
	return created.ID
}

// selected is a Library showing one WidgetType, selected.
func selected(t *testing.T) (*fixture, uuid.UUID) {
	t.Helper()
	url, api := newServer(t)
	id := createWidgetType(t, api, "Gauge")
	f := mount(t, url, api)
	f.l.selectItem(id)
	if f.l.selectedID != id {
		t.Fatal("the widget type is not loaded")
	}
	return f, id
}

func TestCreateItem_SelectsTheNewWidgetType(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)

	f.l.createItem(f.l.compoCtx)
	f.run()

	f.noToasts()
	list, err := api.ListWidgetTypes(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("server widget types: %+v, %v", list, err)
	}
	wt := list[0]
	if wt.Name != "New Widget Type" || wt.HtmlTemplate != defaultHTML || wt.Script != defaultScript || wt.DefaultWidth != 120 {
		t.Errorf("server widget type: %+v", wt)
	}
	if f.l.selectedID != wt.ID || f.l.edit.name != wt.Name || len(f.l.widgetTypes) != 1 {
		t.Errorf("selected %v (%q) of %d", f.l.selectedID, f.l.edit.name, len(f.l.widgetTypes))
	}
}

func TestApplyChanges_RightAfterCreate_SavesTheEdit(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.l.reloader.Start() // a list read in flight, which never lists the new type

	f.l.createItem(f.l.compoCtx)
	f.run()
	f.l.edit.name = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	f.noToasts()
	list, err := api.ListWidgetTypes(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("server widget types: %+v, %v", list, err)
	}
	if wt := list[0]; wt.Name != "Mine" || wt.DefaultWidth != 120 || wt.DefaultHeight != 60 {
		t.Errorf("server widget type: %+v, want the edit with the default size kept", wt)
	}
}

func TestApplyChanges_ListReadBeforeAConflict_DoesNotRestoreTheOldVersion(t *testing.T) {
	f, id := selected(t)
	inFlight, mark := f.l.widgetTypes, f.l.versions.Mark() // read before the conflict
	f.l.versions.Observe(mark, id, 15)                     // the server's database was recreated since

	f.l.edit.name = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()
	f.oneToast() // the conflict
	stale := slices.Clone(inFlight)
	stale[0].Version = 15
	f.l.setWidgetTypes(f.l.compoCtx, mark, stale) // its old answer arrives after the reread

	f.l.applyChanges(f.l.compoCtx)
	f.run()

	f.noToasts()
	if name := f.widgetType(id).Name; name != "Mine" {
		t.Errorf("repeated edit: server name %q", name)
	}
}

func TestDeleteItem_RemovesItAndTheSelection(t *testing.T) {
	f, id := selected(t)

	f.l.deleteItem(f.l.compoCtx)
	f.run()

	f.noToasts()
	if _, err := f.api.GetWidgetType(context.Background(), id); err == nil {
		t.Error("the widget type is still on the server")
	}
	if len(f.l.widgetTypes) != 0 || f.l.selectedID != uuid.Nil {
		t.Errorf("widget types %+v, selected %v: want none", f.l.widgetTypes, f.l.selectedID)
	}
}

// TestRelease_ReleasesTheDraftAndShowsItInTheHistory: Release makes release
// 1 of the Draft, shown in the history; viewing it is read-only.
func TestRelease_ReleasesTheDraftAndShowsItInTheHistory(t *testing.T) {
	f, id := selected(t)

	if lib, ok := f.l.selectedLibrary(); !ok || !lib.HasUnreleasedChanges || lib.LatestRelease != 0 {
		t.Fatalf("library before the release: %+v", lib)
	}
	releaseNow(t, f)

	if lib, _ := f.l.selectedLibrary(); lib.LatestRelease != 1 || lib.HasUnreleasedChanges {
		t.Errorf("library after the release: %+v", lib)
	}
	if len(f.l.releases) != 1 || len(f.l.releases[0].WidgetTypes) != 1 || f.l.releases[0].WidgetTypes[0].ID != id {
		t.Fatalf("history: %+v", f.l.releases)
	}
	f.l.view(1)
	if !f.l.readOnly() || len(f.l.shownTypes()) != 1 {
		t.Errorf("viewing release 1: read-only %t, %d types", f.l.readOnly(), len(f.l.shownTypes()))
	}
	f.l.createItem(f.l.compoCtx)
	f.run()
	if list, _ := f.api.ListWidgetTypes(context.Background()); len(list) != 1 {
		t.Errorf("an edit while viewing a release reached the server: %+v", list)
	}
	f.l.view(0)
	if f.l.readOnly() {
		t.Error("the Draft is editable again")
	}
}

// releaseNow releases the selected library's Draft as Release does, without
// its dialog.
func releaseNow(t *testing.T, f *fixture) {
	t.Helper()
	lib, _ := f.l.selectedLibrary()
	if _, err := f.api.Release(context.Background(), apiv0.CreateReleaseRequest{Version: lib.Version}); err != nil {
		t.Fatalf("release: %v", err)
	}
	f.l.loadLibraries(f.l.compoCtx)
	f.l.loadReleases(f.l.compoCtx)
	f.run()
	f.noToasts()
}

// TestDiscard_MakesTheDraftReadOnlyWhileUnderWay: once the server marks the
// Discard, the Draft cannot be edited until it ends.
func TestDiscard_MakesTheDraftReadOnlyWhileUnderWay(t *testing.T) {
	f, _ := selected(t)
	releaseNow(t, f)
	createWidgetType(t, f.api, "Added")
	if _, err := f.api.Discard(context.Background()); err != nil {
		t.Fatalf("discard: %v", err)
	}
	f.l.loadLibraries(f.l.compoCtx)
	f.run()

	lib, _ := f.l.selectedLibrary()
	if lib.DiscardingTo == 0 {
		// The server may have finished the Discard already.
		t.Skip("the Discard ended before the library was read")
	}
	if !f.l.discarding() || !f.l.readOnly() {
		t.Error("a Draft being discarded must be read-only")
	}
}

func TestLibraries_SelectFallsBackToTheFirst(t *testing.T) {
	url, api := newServer(t)
	other, err := api.CreateLibrary(context.Background(), apiv0.CreateLibraryRequest{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	f := mount(t, url, api)
	f.run()
	if f.l.libraryID != api.ID() || len(f.l.libraries) != 2 {
		t.Fatalf("selected %v of %d libraries, want the first", f.l.libraryID, len(f.l.libraries))
	}
	f.l.selectLibrary(f.l.compoCtx, other.ID)
	f.run()
	if f.l.libraryID != other.ID || len(f.l.widgetTypes) != 0 {
		t.Errorf("the other library: %v, %d types", f.l.libraryID, len(f.l.widgetTypes))
	}
	if _, err := api.DeleteLibrary(context.Background(), other.ID); err != nil {
		t.Fatal(err)
	}
	f.l.loadLibraries(f.l.compoCtx)
	f.run()
	if f.l.libraryID != api.ID() {
		t.Errorf("after the deletion selected %v, want the first", f.l.libraryID)
	}
}

// TestLibraries_SelectedDeleted_SelectsAnotherWithoutAnError: the Draft and
// the releases of a library deleted elsewhere answer 404 before the list is
// reloaded; that selects another library instead of showing an error, and
// none once the last is gone.
func TestLibraries_SelectedDeleted_SelectsAnotherWithoutAnError(t *testing.T) {
	url, api := newServer(t)
	other, err := api.CreateLibrary(context.Background(), apiv0.CreateLibraryRequest{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	f := mount(t, url, api)
	f.run()
	f.l.selectLibrary(f.l.compoCtx, other.ID)
	f.run()
	if _, err := api.DeleteLibrary(context.Background(), other.ID); err != nil {
		t.Fatal(err)
	}
	f.l.loadList(f.l.compoCtx)
	f.l.loadReleases(f.l.compoCtx)
	f.run()
	f.noToasts()
	if f.l.libraryID != api.ID() {
		t.Errorf("after the deletion selected %v, want %v", f.l.libraryID, api.ID())
	}

	if _, err := api.DeleteLibrary(context.Background(), api.ID()); err != nil {
		t.Fatal(err)
	}
	f.l.loadList(f.l.compoCtx)
	f.run()
	f.noToasts()
	if f.l.libraryID != uuid.Nil || f.l.widgetTypes != nil || f.l.releases != nil {
		t.Errorf("no library left, yet selected %v with %d types and %d releases",
			f.l.libraryID, len(f.l.widgetTypes), len(f.l.releases))
	}
}

func TestApplyChanges_SavesTheEditor(t *testing.T) {
	f, id := selected(t)

	f.l.edit.name = "Big gauge"
	f.l.edit.html = "<span></span>"
	f.l.edit.script = "function render(inputs) { return 1 }"
	f.l.edit.ports = append(f.l.edit.ports, apiv0.InputPort{Name: "on", TypeHint: "boolean"})
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	f.noToasts()
	wt := f.widgetType(id)
	if wt.Name != "Big gauge" || wt.HtmlTemplate != "<span></span>" || wt.Script != f.l.edit.script ||
		len(wt.InputPorts) != 2 || wt.DefaultWidth != 150 {
		t.Errorf("server widget type: %+v", wt)
	}
	if got := f.l.versions.Of(id); got != wt.Version {
		t.Errorf("known version %d, server's %d", got, wt.Version)
	}

	// The next save starts from the version just written.
	f.l.edit.name = "Bigger gauge"
	f.l.applyChanges(f.l.compoCtx)
	f.run()
	f.noToasts()
	if name := f.widgetType(id).Name; name != "Bigger gauge" {
		t.Errorf("second save: server name %q", name)
	}
}

func TestApplyChanges_ChangedMeanwhile_ShowsTheServersConflict(t *testing.T) {
	f, id := selected(t)
	wt := f.widgetType(id)
	if _, err := f.api.UpdateWidgetType(context.Background(), id, apiv0.UpdateWidgetTypeRequest{
		Name: "Theirs", HtmlTemplate: wt.HtmlTemplate, Script: wt.Script, ScriptLanguage: wt.ScriptLanguage,
		DefaultWidth: wt.DefaultWidth, DefaultHeight: wt.DefaultHeight, InputPorts: wt.InputPorts, Version: wt.Version,
	}); err != nil {
		t.Fatal(err)
	}

	f.l.edit.name = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != http.StatusConflict || got.Method != http.MethodPut || !strings.Contains(got.Detail, "changed concurrently") {
		t.Errorf("toast %+v: want the server's 409", got)
	}
	if name := f.widgetType(id).Name; name != "Theirs" {
		t.Errorf("server name %q: the refused write changed it", name)
	}
}

func TestApplyChanges_StaleListAfterTheSave_KeepsTheSavedVersion(t *testing.T) {
	f, id := selected(t)
	stale, mark := f.l.widgetTypes, f.l.versions.Mark() // a list read before the save, answered after it

	f.l.edit.name = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()
	f.l.setWidgetTypes(f.l.compoCtx, mark, stale)

	f.l.edit.name = "Mine again"
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	f.noToasts()
	if name := f.widgetType(id).Name; name != "Mine again" {
		t.Errorf("second save: server name %q", name)
	}
}

func TestApplyChanges_ServerVersionWentBack_RecoversAfterTheConflict(t *testing.T) {
	f, id := selected(t)
	f.l.versions.Observe(f.l.versions.Mark(), id, 15) // the server's database was recreated since

	f.l.edit.name = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	if got := f.oneToast(); got.Status != http.StatusConflict {
		t.Errorf("toast %+v: want the conflict", got)
	}
	if got, want := f.l.versions.Of(id), f.widgetType(id).Version; got != want {
		t.Errorf("known version %d, server's %d", got, want)
	}

	f.l.edit.name = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	f.noToasts()
	if name := f.widgetType(id).Name; name != "Mine" {
		t.Errorf("repeated edit: server name %q", name)
	}
}

func TestApplyChanges_Refused_ShowsTheServersDetail(t *testing.T) {
	f, _ := selected(t)

	f.l.edit.ports = append(f.l.edit.ports, apiv0.InputPort{Name: "value", TypeHint: "integer"})
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != http.StatusBadRequest || got.Title != "Bad Request" || got.Detail == "" {
		t.Errorf("toast %+v: want the server's 400 with its detail", got)
	}
}

func TestCommitEdit_RenamesTheWidgetType(t *testing.T) {
	f, id := selected(t)
	f.l.edit.html = "<p>unsaved</p>"

	f.l.startEditing(id, "Gauge")
	f.l.editingName = "Dial"
	f.l.commitEdit(f.l.compoCtx)
	f.run()

	f.noToasts()
	wt := f.widgetType(id)
	if wt.Name != "Dial" || wt.HtmlTemplate != "<div></div>" {
		t.Errorf("server widget type %+v: want only the name changed", wt)
	}
	if f.l.editingID != uuid.Nil || f.l.edit.name != "Dial" || f.l.edit.html != "<p>unsaved</p>" {
		t.Errorf("editor: editing %v, name %q, html %q", f.l.editingID, f.l.edit.name, f.l.edit.html)
	}
}

func TestNoServer_ShowsANetworkError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + l.Addr().String()
	l.Close()

	f := mount(t, url, nil)

	if got := f.oneToast(); got.Title != "Network Error" || got.Status != 0 || !strings.Contains(got.Detail, l.Addr().String()) {
		t.Errorf("toast %+v: want a Network Error naming the server", got)
	}
	if f.l.reloader.Loading() {
		t.Error("still loading after the failure")
	}
}
