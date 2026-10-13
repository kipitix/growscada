package library

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

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
	api *apiclient.Client
}

func newServer(t *testing.T) (string, *apiclient.Client) {
	t.Helper()
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	return srv.URL, apiclient.New(srv.URL, srv.Client())
}

func mount(t *testing.T, url string, api *apiclient.Client) *fixture {
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

func createWidgetType(t *testing.T, api *apiclient.Client, name string) uuid.UUID {
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
	if f.l.selectedID != wt.ID || f.l.editedName != wt.Name || len(f.l.widgetTypes) != 1 {
		t.Errorf("selected %v (%q) of %d", f.l.selectedID, f.l.editedName, len(f.l.widgetTypes))
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

func TestDeleteItem_InUse_ShowsTheServersRefusal(t *testing.T) {
	f, id := selected(t)
	ctx := context.Background()
	scene, err := f.api.CreateScene(ctx, apiv0.CreateSceneRequest{Name: "Main", Width: 800, Height: 600})
	if err != nil {
		t.Fatal(err)
	}
	sc, _ := f.api.GetScene(ctx, scene.ID)
	if _, err := f.api.CreateWidget(ctx, scene.ID, apiv0.CreateWidgetRequest{
		Name: "w", Size: apiv0.SizeRequest{Width: 10, Height: 10}, Origin: apiv0.OriginRequest{X: 0.5, Y: 0.5},
		TypeID: id, SceneVersion: sc.Version, Labels: []string{}, PortBindings: []apiv0.PortBinding{},
	}); err != nil {
		t.Fatal(err)
	}

	f.l.deleteItem(f.l.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != http.StatusConflict || got.Method != http.MethodDelete || got.Title != "Conflict: WidgetType" ||
		got.Detail != "WidgetType is used by Widgets" {
		t.Errorf("toast %+v: want the server's 409", got)
	}
	if f.l.selectedID != id {
		t.Error("the selection was dropped")
	}
}

func TestApplyChanges_SavesTheEditor(t *testing.T) {
	f, id := selected(t)

	f.l.editedName = "Big gauge"
	f.l.editedHTML = "<span></span>"
	f.l.editedScript = "function render(inputs) { return 1 }"
	f.l.editedInputPorts = append(f.l.editedInputPorts, apiv0.InputPort{Name: "on", TypeHint: "boolean"})
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	f.noToasts()
	wt := f.widgetType(id)
	if wt.Name != "Big gauge" || wt.HtmlTemplate != "<span></span>" || wt.Script != f.l.editedScript ||
		len(wt.InputPorts) != 2 || wt.DefaultWidth != 150 {
		t.Errorf("server widget type: %+v", wt)
	}
	if got := f.l.versions.Of(id); got != wt.Version {
		t.Errorf("known version %d, server's %d", got, wt.Version)
	}

	// The next save starts from the version just written.
	f.l.editedName = "Bigger gauge"
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

	f.l.editedName = "Mine"
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
	stale := f.l.widgetTypes // a list read before the save, answered after it

	f.l.editedName = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()
	f.l.setWidgetTypes(f.l.compoCtx, stale)

	f.l.editedName = "Mine again"
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	f.noToasts()
	if name := f.widgetType(id).Name; name != "Mine again" {
		t.Errorf("second save: server name %q", name)
	}
}

func TestApplyChanges_ServerVersionWentBack_RecoversAfterTheConflict(t *testing.T) {
	f, id := selected(t)
	f.l.versions.Observe(id, 15) // the server's database was recreated since

	f.l.editedName = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	if got := f.oneToast(); got.Status != http.StatusConflict {
		t.Errorf("toast %+v: want the conflict", got)
	}
	if got, want := f.l.versions.Of(id), f.widgetType(id).Version; got != want {
		t.Errorf("known version %d, server's %d", got, want)
	}

	f.l.editedName = "Mine"
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	f.noToasts()
	if name := f.widgetType(id).Name; name != "Mine" {
		t.Errorf("repeated edit: server name %q", name)
	}
}

func TestApplyChanges_Refused_ShowsTheServersDetail(t *testing.T) {
	f, _ := selected(t)

	f.l.editedInputPorts = append(f.l.editedInputPorts, apiv0.InputPort{Name: "value", TypeHint: "integer"})
	f.l.applyChanges(f.l.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != http.StatusBadRequest || got.Title != "Bad Request" || got.Detail == "" {
		t.Errorf("toast %+v: want the server's 400 with its detail", got)
	}
}

func TestCommitEdit_RenamesTheWidgetType(t *testing.T) {
	f, id := selected(t)
	f.l.editedHTML = "<p>unsaved</p>"

	f.l.startEditing(id, "Gauge")
	f.l.editingName = "Dial"
	f.l.commitEdit(f.l.compoCtx)
	f.run()

	f.noToasts()
	wt := f.widgetType(id)
	if wt.Name != "Dial" || wt.HtmlTemplate != "<div></div>" {
		t.Errorf("server widget type %+v: want only the name changed", wt)
	}
	if f.l.editingID != uuid.Nil || f.l.editedName != "Dial" || f.l.editedHTML != "<p>unsaved</p>" {
		t.Errorf("editor: editing %v, name %q, html %q", f.l.editingID, f.l.editedName, f.l.editedHTML)
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
