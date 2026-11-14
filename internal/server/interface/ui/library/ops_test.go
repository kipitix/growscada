package library

import (
	"context"
	"errors"
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
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
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
	url string
}

func newServer(t *testing.T) (string, *testAPI) {
	t.Helper()
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	return srv.URL, newTestAPI(t, apiclient.New(srv.URL, srv.Client()))
}

// mount mounts a Library showing the library the test API works on: Root
// announces the selection of its page with SetLibrary (task 73), which a test
// does here directly. The actions named watch are recorded (TakeActions).
func mount(t *testing.T, url string, api *testAPI, watch ...string) *fixture {
	t.Helper()
	l := NewLibrary(apiclient.New(url, http.DefaultClient))
	f := &fixture{t: t, l: l, api: api, url: url}
	f.e = uitest.LoadWatching(t, mountOnHost{l}, watch...)
	if api != nil {
		f.l.SetLibrary(api.ID())
		f.e.ConsumeAll()
	}
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
	inFlight, mark := f.l.widgetTypes, f.l.markDraft() // read before the conflict
	f.l.observeDraftVersion(mark, 15)                  // the server's database was recreated since

	f.l.edit.name = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()
	f.oneToast() // the conflict
	stale := slices.Clone(inFlight)
	stale[0].Version = 15
	f.l.setWidgetTypes(f.l.compoCtx, mark, 0, stale) // its old answer arrives after the reread

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

// TestDeleteItem_LastType_KeepsTheDraftVersion: deleting the last WidgetType
// leaves no content DTO carrying the Draft's Version, so the mode rereads it
// (ADR 0016): the next write states the server's one, not the one before the
// deletion, and is not refused as stale.
func TestDeleteItem_LastType_KeepsTheDraftVersion(t *testing.T) {
	f, _ := selected(t)
	saveNow(t, f)
	f.l.deleteItem(f.l.compoCtx)
	f.run()
	f.noToasts()

	if err := saveAndCommit(context.Background(), f.api.LibraryClient); err != nil {
		t.Fatalf("save of the emptied Draft at the known version: %v", err)
	}
}

// saveAndCommit saves the library's Draft as a Commit, at the Version the
// server states. A Draft already at its last Commit keeps that one.
func saveAndCommit(ctx context.Context, lib *apiclient.LibraryClient) error {
	state, err := lib.DraftState(ctx)
	if err != nil {
		return err
	}
	_, err = lib.SaveCommit(ctx, apiv0.CreateCommitRequest{Version: state.Version, Title: "Save", Author: "Ada"})
	if err == nil || !errors.Is(err, apiclient.ErrConflict) {
		return err
	}
	return nil
}

// saveNow saves the selected library's Draft as a saved state and rereads what
// the Save changes.
func saveNow(t *testing.T, f *fixture) {
	t.Helper()
	if err := saveAndCommit(context.Background(), f.api.LibraryClient); err != nil {
		t.Fatalf("save: %v", err)
	}
	f.l.loadLibraries(f.l.compoCtx)
	f.l.loadCommits(f.l.compoCtx)
	f.l.loadDraftState(f.l.compoCtx)
	f.run()
	f.noToasts()
}

// TestDiscard_ResetsTheDraftAtOnce: a Discard is one transaction, so the
// Draft is back at its last saved state as soon as the call returns.
func TestDiscard_ResetsTheDraftAtOnce(t *testing.T) {
	f, kept := selected(t)
	saveNow(t, f)
	createWidgetType(t, f.api, "Added")
	state, err := f.api.Discard(context.Background())
	if err != nil {
		t.Fatalf("discard: %v", err)
	}
	if state.Version <= 0 {
		t.Errorf("the Draft after the Discard: %+v", state)
	}
	list, err := f.api.ListWidgetTypes(context.Background())
	if err != nil {
		t.Fatalf("list widget types: %v", err)
	}
	if len(list) != 1 || list[0].ID != kept {
		t.Errorf("the Draft after the Discard: %+v, want only the saved type", list)
	}
}

// TestLibraries_SetLibraryShowsTheGivenOne: the page announces the library,
// not the column: SetLibrary shows the given one and drops what was shown of
// another (task 73). A library deleted elsewhere is left through the action
// of any read of it (TestLibraryGone_AnyReadOfTheSelectedLibraryFiresTheAction).
func TestLibraries_SetLibraryShowsTheGivenOne(t *testing.T) {
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
	f.l.SetLibrary(other.ID)
	f.run()
	if f.l.libraryID != other.ID || len(f.l.widgetTypes) != 0 {
		t.Errorf("the other library: %v, %d types", f.l.libraryID, len(f.l.widgetTypes))
	}
}

// TestLibraryGone_AnyReadOfTheSelectedLibraryFiresTheAction: a 404 of any of
// the reads of the selected library — its Draft's WidgetTypes, the state of
// the Draft, its stack of DraftChanges, its saved states — means the
// library is gone: the read fires ActionLibraryGone with the library's ID and
// shows no toast (task 76). Root leaves the library and shows the page on it.
func TestLibraryGone_AnyReadOfTheSelectedLibraryFiresTheAction(t *testing.T) {
	reads := []struct {
		name string
		load func(*fixture)
	}{
		{"widget types", func(f *fixture) { f.l.loadList(f.l.compoCtx) }},
		{"draft state", func(f *fixture) { f.l.loadDraftState(f.l.compoCtx) }},
		{"draft changes", func(f *fixture) { f.l.loadDraftChanges(f.l.compoCtx) }},
		{"commits", func(f *fixture) { f.l.loadCommits(f.l.compoCtx) }},
	}
	for _, read := range reads {
		t.Run(read.name, func(t *testing.T) {
			url, api := newServer(t)
			f := mount(t, url, api, uiutil.ActionLibraryGone)
			f.run()
			gone := api.ID()
			if _, err := api.DeleteLibrary(context.Background(), gone); err != nil {
				t.Fatal(err)
			}
			f.e.TakeActions()

			read.load(f)
			f.run()

			f.noToasts()
			got := f.e.TakeActions()
			if len(got) != 1 || got[0].Name != uiutil.ActionLibraryGone {
				t.Fatalf("actions %+v: want one %q", got, uiutil.ActionLibraryGone)
			}
			if id, ok := got[0].Value.(uuid.UUID); !ok || id != gone {
				t.Errorf("the action names %v, want the library %v", got[0].Value, gone)
			}
		})
	}
}

// TestLibraryGone_ACommandKeepsItsOwnError: a command's 404 names what the
// command could not find, not the library: the editor stays on the library and
// the server's error is shown, and no read reported it gone (task 76).
func TestLibraryGone_ACommandKeepsItsOwnError(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api, uiutil.ActionLibraryGone)
	saveNow(t, f)
	gone := api.ID()
	if _, err := api.DeleteLibrary(context.Background(), gone); err != nil {
		t.Fatal(err)
	}
	f.e.TakeActions()

	f.l.renameSavedState(f.l.compoCtx, 1, "Renamed")
	f.run()

	if got := f.oneToast(); got.Status != http.StatusNotFound {
		t.Errorf("toast %+v: want the server's 404", got)
	}
	if f.l.libraryID != gone {
		t.Errorf("the editor left the library %v for %v on a command's 404", gone, f.l.libraryID)
	}
	if got := f.e.TakeActions(); len(got) != 0 {
		t.Errorf("a command fired %+v: only a read reports the library gone", got)
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
	if got := f.l.draftVersion(); got != wt.Version {
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
	stale, mark := f.l.widgetTypes, f.l.markDraft() // a list read before the save, answered after it

	f.l.edit.name = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()
	f.l.setWidgetTypes(f.l.compoCtx, mark, 0, stale)

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
	f.l.observeDraftVersion(f.l.markDraft(), 15) // the server's database was recreated since

	f.l.edit.name = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	if got := f.oneToast(); got.Status != http.StatusConflict {
		t.Errorf("toast %+v: want the conflict", got)
	}
	if got, want := f.l.draftVersion(), f.widgetType(id).Version; got != want {
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
