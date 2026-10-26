package project

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
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

// The operations of Project run against the real REST API over Postgres:
// a fake would have to repeat the server's rules (Scene versions, 409,
// validation, reconciliation).

var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// mountOnHost lets go-app mount a Project on the host (see uitest).
type mountOnHost struct{ *Project }

func (m mountOnHost) OnPreRender(ctx app.Context) { m.OnMount(ctx) }

// fixture is a mounted Project and a second client of the same server, which
// arranges and checks the server's state as another user would.
type fixture struct {
	t   *testing.T
	p   *Project
	e   *uitest.Engine
	api *testAPI
}

// newServer starts the API and returns a client of it for arranging.
func newServer(t *testing.T) (string, *testAPI) {
	t.Helper()
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	return srv.URL, newTestAPI(apiclient.New(srv.URL, srv.Client()))
}

// mount mounts a Project talking to the server at url.
func mount(t *testing.T, url string, api *testAPI) *fixture {
	t.Helper()
	return mountWith(t, url, api, http.DefaultClient)
}

// mountWith is mount with the Project's requests sent through httpClient.
func mountWith(t *testing.T, url string, api *testAPI, httpClient *http.Client) *fixture {
	t.Helper()
	p := NewProject(apiclient.New(url, httpClient))
	p.SetProject(servertest.MainProject)
	f := &fixture{t: t, p: p, api: api}
	f.e = uitest.Load(t, mountOnHost{p})
	return f
}

func startProject(t *testing.T) *fixture {
	t.Helper()
	url, api := newServer(t)
	return mount(t, url, api)
}

// run runs everything the last operation started.
func (f *fixture) run() { f.e.ConsumeAll() }

// noToasts fails if a toast was shown.
func (f *fixture) noToasts() {
	f.t.Helper()
	if toasts := f.e.TakeToasts(); len(toasts) > 0 {
		f.t.Fatalf("unexpected toasts: %+v", toasts)
	}
}

// oneToast returns the only toast shown since the last check.
func (f *fixture) oneToast() toast.Problem {
	f.t.Helper()
	toasts := f.e.TakeToasts()
	if len(toasts) != 1 {
		f.t.Fatalf("want one toast, got %+v", toasts)
	}
	return toasts[0]
}

func (f *fixture) scene(id uuid.UUID) apiv0.SceneResponse {
	f.t.Helper()
	sc, err := f.api.GetScene(context.Background(), id)
	if err != nil {
		f.t.Fatalf("get scene: %v", err)
	}
	return sc
}

func (f *fixture) widget(sceneID, id uuid.UUID) apiv0.WidgetResponse {
	f.t.Helper()
	w, err := f.api.GetWidget(context.Background(), sceneID, id)
	if err != nil {
		f.t.Fatalf("get widget: %v", err)
	}
	return w
}

// knowsSceneVersion fails unless Project's version of the Scene is the
// server's.
func (f *fixture) knowsSceneVersion(id uuid.UUID) {
	f.t.Helper()
	if got, want := f.p.sceneVersions.Of(id), f.scene(id).Version; got != want {
		f.t.Errorf("known version of the Scene: %d, server's: %d", got, want)
	}
}

// ── Arranging the server's state ─────────────────────────────────────────────

func createScene(t *testing.T, api *testAPI, name string) uuid.UUID {
	t.Helper()
	created, err := api.CreateScene(context.Background(), apiv0.CreateSceneRequest{Name: name, Width: 800, Height: 600})
	if err != nil {
		t.Fatalf("create scene: %v", err)
	}
	return created.ID
}

func createWidgetType(t *testing.T, api *testAPI, name string, ports ...apiv0.InputPort) uuid.UUID {
	t.Helper()
	if ports == nil {
		ports = []apiv0.InputPort{}
	}
	created, err := api.CreateWidgetType(context.Background(), apiv0.CreateWidgetTypeRequest{
		Name: name, HtmlTemplate: "<div></div>", ScriptLanguage: "javascript",
		DefaultWidth: 150, DefaultHeight: 70, InputPorts: ports,
	})
	if err != nil {
		t.Fatalf("create widget type: %v", err)
	}
	return created.ID
}

func createTag(t *testing.T, api *testAPI, name, tagType string) uuid.UUID {
	t.Helper()
	created, err := api.CreateTag(context.Background(), apiv0.CreateTagRequest{Name: name, Type: tagType})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	return created.ID
}

func createWidget(t *testing.T, api *testAPI, sceneID, typeID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	sc, err := api.GetScene(ctx, sceneID)
	if err != nil {
		t.Fatalf("get scene: %v", err)
	}
	created, err := api.CreateWidget(ctx, sceneID, apiv0.CreateWidgetRequest(widgetRequest(apiv0.WidgetResponse{
		Name: name, Position: apiv0.PositionResponse{X: 10, Y: 20}, Size: apiv0.SizeResponse{Width: 100, Height: 50},
		Origin: apiv0.OriginResponse{X: 0.5, Y: 0.5}, TypeID: typeID,
	}, sc.Version)))
	if err != nil {
		t.Fatalf("create widget: %v", err)
	}
	return created.ID
}

// ── Scenes ───────────────────────────────────────────────────────────────────

func TestCreateScene_SelectsTheNewScene(t *testing.T) {
	f := startProject(t)

	f.p.createScene(f.p.compoCtx)
	f.run()

	f.noToasts()
	scenes, err := f.api.ListScenes(context.Background())
	if err != nil || len(scenes) != 1 {
		t.Fatalf("server scenes: %+v, %v", scenes, err)
	}
	if f.p.selectedSceneID != scenes[0].ID || len(f.p.scenes) != 1 {
		t.Errorf("selected %v of %d scenes, want the new %v", f.p.selectedSceneID, len(f.p.scenes), scenes[0].ID)
	}
	f.knowsSceneVersion(scenes[0].ID)
}

func TestSaveSceneProperties_RightAfterCreate_SavesTheEdit(t *testing.T) {
	f := startProject(t)
	f.p.scenesReloader.Start() // a list read in flight, which never lists the new scene

	f.p.createScene(f.p.compoCtx)
	f.run()
	f.p.sceneEdit.name = "Mine"
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	f.noToasts()
	scenes, err := f.api.ListScenes(context.Background())
	if err != nil || len(scenes) != 1 {
		t.Fatalf("server scenes: %+v, %v", scenes, err)
	}
	if sc := scenes[0]; sc.Name != "Mine" || sc.Width != 1920 {
		t.Errorf("server scene: %+v, want the edit", sc)
	}
}

func TestDeleteScene_RemovesIt(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	f := mount(t, url, api)
	if f.p.selectedSceneID != sceneID {
		t.Fatalf("the only scene is not selected: %v", f.p.selectedSceneID)
	}

	f.p.deleteScene(f.p.compoCtx, sceneID)
	f.run()

	f.noToasts()
	if _, err := api.GetScene(context.Background(), sceneID); err == nil {
		t.Error("the scene is still on the server")
	}
	if len(f.p.scenes) != 0 || f.p.selectedSceneID != uuid.Nil {
		t.Errorf("scenes %+v, selected %v: want none", f.p.scenes, f.p.selectedSceneID)
	}
}

func TestConfirmDeleteScene_Declined_KeepsTheScene(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	f := mount(t, url, api)

	f.p.confirmDeleteScene(f.p.compoCtx, sceneID, "Main") // no browser: confirm() is false
	f.run()

	f.noToasts()
	f.scene(sceneID)
}

func TestSaveSceneProperties_SavesTheEditor(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	f := mount(t, url, api)

	f.p.sceneEdit.name = "Overview"
	f.p.sceneEdit.width = "1024"
	f.p.sceneEdit.height = "768"
	f.p.sceneEdit.background = "<b>bg</b>"
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	f.noToasts()
	sc := f.scene(sceneID)
	if sc.Name != "Overview" || sc.Width != 1024 || sc.Height != 768 || sc.BackgroundHTML != "<b>bg</b>" {
		t.Errorf("server scene: %+v", sc)
	}
	f.knowsSceneVersion(sceneID)
}

func TestCommitSceneEdit_RenamesTheScene(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	f := mount(t, url, api)

	f.p.startEditingScene(sceneID, "Main")
	f.p.editingSceneName = "Renamed"
	f.p.commitSceneEdit(f.p.compoCtx)
	f.run()

	f.noToasts()
	if sc := f.scene(sceneID); sc.Name != "Renamed" || sc.Width != 800 {
		t.Errorf("server scene: %+v", sc)
	}
	if f.p.editingSceneID != uuid.Nil || f.p.sceneEdit.name != "Renamed" {
		t.Errorf("editor: editing %v, name %q", f.p.editingSceneID, f.p.sceneEdit.name)
	}
	f.knowsSceneVersion(sceneID)
}

// A second client edits the Scene between Project's read and its write: the
// write is refused, the user is asked to repeat the edit, the Scene is reread
// and the repeated edit goes through.
func TestSaveSceneProperties_SceneChangedMeanwhile_ReloadsAndAsksToRepeat(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	f := mount(t, url, api)
	sc := f.scene(sceneID)
	if _, err := api.UpdateScene(context.Background(), sceneID, apiv0.UpdateSceneRequest{
		Name: "Theirs", Width: sc.Width, Height: sc.Height, Version: sc.Version,
	}); err != nil {
		t.Fatal(err)
	}

	f.p.sceneEdit.name = "Mine"
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Title != "Scene changed" || got.Status != http.StatusConflict || got.Method != http.MethodPut {
		t.Errorf("toast: %+v", got)
	}
	if name := f.scene(sceneID).Name; name != "Theirs" {
		t.Errorf("server name %q: the refused write changed it", name)
	}
	if f.p.scenes[0].Name != "Theirs" || f.p.sceneEdit.name != "Theirs" {
		t.Errorf("not reloaded: scene %q, editor %q", f.p.scenes[0].Name, f.p.sceneEdit.name)
	}
	f.knowsSceneVersion(sceneID)

	f.p.sceneEdit.name = "Mine"
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	f.noToasts()
	if name := f.scene(sceneID).Name; name != "Mine" {
		t.Errorf("repeated edit: server name %q", name)
	}
}

func TestSaveSceneProperties_ServerVersionWentBack_RecoversAfterTheConflict(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	f := mount(t, url, api)
	f.p.sceneVersions.Observe(f.p.sceneVersions.Mark(), sceneID, 15) // the server's database was recreated since

	f.p.sceneEdit.name = "Mine"
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	if got := f.oneToast(); got.Status != http.StatusConflict {
		t.Errorf("toast %+v: want the conflict", got)
	}
	f.knowsSceneVersion(sceneID)

	f.p.sceneEdit.name = "Mine"
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	f.noToasts()
	if name := f.scene(sceneID).Name; name != "Mine" {
		t.Errorf("repeated edit: server name %q", name)
	}
}

func TestSaveSceneProperties_Refused_ShowsTheServersDetailAndPutsItsStateBack(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	f := mount(t, url, api)

	f.p.sceneEdit.width = "-5"
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != http.StatusBadRequest || got.Title != "Bad Request" || got.Method != http.MethodPut ||
		!strings.Contains(got.Detail, "width must be positive") {
		t.Errorf("toast %+v: want the server's 400 with its detail", got)
	}
	if f.p.sceneEdit.width != "800" || f.p.scenes[0].Width != 800 {
		t.Errorf("the server's width is not back: editor %q, scene %d", f.p.sceneEdit.width, f.p.scenes[0].Width)
	}
	if w := f.scene(sceneID).Width; w != 800 {
		t.Errorf("server width %d", w)
	}
}

// ── Widgets ──────────────────────────────────────────────────────────────────

// widgetFixture is a Project showing a Scene with one Widget, selected.
func widgetFixture(t *testing.T, ports ...apiv0.InputPort) (*fixture, uuid.UUID, uuid.UUID) {
	t.Helper()
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	typeID := createWidgetType(t, api, "Gauge", ports...)
	widgetID := createWidget(t, api, sceneID, typeID, "gauge1")
	f := mount(t, url, api)
	f.p.selectWidget(f.p.compoCtx, widgetID)
	if f.p.selectedWidgetIdx() < 0 {
		t.Fatal("the widget is not loaded")
	}
	return f, sceneID, widgetID
}

func TestCreateWidget_PlacesTheWidgetTypeAndSelectsIt(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	typeID := createWidgetType(t, api, "Gauge")
	f := mount(t, url, api)

	f.p.createWidget(f.p.compoCtx, typeID, 30, 40)
	f.run()

	f.noToasts()
	widgets, err := api.ListWidgets(context.Background(), sceneID)
	if err != nil || len(widgets) != 1 {
		t.Fatalf("server widgets: %+v, %v", widgets, err)
	}
	w := widgets[0]
	if w.Name != "Gauge" || w.TypeID != typeID || w.Size != (apiv0.SizeResponse{Width: 150, Height: 70}) ||
		w.Position.X != 30 || w.Position.Y != 40 || w.Origin != (apiv0.OriginResponse{X: 0.5, Y: 0.5}) {
		t.Errorf("server widget: %+v", w)
	}
	if f.p.selectedWidgetID != w.ID || len(f.p.widgets) != 1 {
		t.Errorf("selected %v of %d widgets, want the new %v", f.p.selectedWidgetID, len(f.p.widgets), w.ID)
	}
	if got := f.p.widgetEdit; got != widgetEditFieldsOf(w) {
		t.Errorf("editor %+v, want %+v", got, widgetEditFieldsOf(w))
	}
	f.knowsSceneVersion(sceneID)
}

func TestCreateWidget_SceneChangedMeanwhile_ReloadsAndAsksToRepeat(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	typeID := createWidgetType(t, api, "Gauge")
	f := mount(t, url, api)
	theirs := createWidget(t, api, sceneID, typeID, "theirs")

	f.p.createWidget(f.p.compoCtx, typeID, 30, 40)
	f.run()

	got := f.oneToast()
	if got.Title != "Scene changed" || got.Status != http.StatusConflict || got.Method != http.MethodPost {
		t.Errorf("toast: %+v", got)
	}
	if len(f.p.widgets) != 1 || f.p.widgets[0].ID != theirs {
		t.Errorf("widgets not reloaded: %+v", f.p.widgets)
	}
	f.knowsSceneVersion(sceneID)

	f.p.createWidget(f.p.compoCtx, typeID, 30, 40)
	f.run()

	f.noToasts()
	if len(f.p.widgets) != 2 {
		t.Errorf("repeated create: %d widgets", len(f.p.widgets))
	}
}

func TestDeleteWidget_RemovesItAndTheSelection(t *testing.T) {
	f, sceneID, widgetID := widgetFixture(t)

	f.p.deleteWidget(f.p.compoCtx, widgetID)
	f.run()

	f.noToasts()
	if _, err := f.api.GetWidget(context.Background(), sceneID, widgetID); err == nil {
		t.Error("the widget is still on the server")
	}
	if len(f.p.widgets) != 0 || f.p.selectedWidgetID != uuid.Nil {
		t.Errorf("widgets %+v, selected %v: want none", f.p.widgets, f.p.selectedWidgetID)
	}
	f.knowsSceneVersion(sceneID)
}

// okOnDelete answers a successful DELETE with a proxy's "OK" in place of the
// server's body.
type okOnDelete struct{}

func (okOnDelete) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil || r.Method != http.MethodDelete || resp.StatusCode != http.StatusOK {
		return resp, err
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(strings.NewReader("OK"))
	resp.ContentLength = 2
	return resp, nil
}

func TestDeleteWidget_ResponseWithoutTheVersion_RereadsIt(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	typeID := createWidgetType(t, api, "Gauge")
	widgetID := createWidget(t, api, sceneID, typeID, "gauge1")
	f := mountWith(t, url, api, &http.Client{Transport: okOnDelete{}})

	f.p.deleteWidget(f.p.compoCtx, widgetID)
	f.run()

	f.noToasts()
	f.knowsSceneVersion(sceneID)
}

func TestSaveWidgetName_RenamesTheWidget(t *testing.T) {
	f, sceneID, widgetID := widgetFixture(t)

	f.p.widgetEdit.name = "pump"
	f.p.saveWidgetName(f.p.compoCtx)
	f.run()

	f.noToasts()
	if w := f.widget(sceneID, widgetID); w.Name != "pump" || w.Position.X != 10 {
		t.Errorf("server widget: %+v", w)
	}
	f.knowsSceneVersion(sceneID)
}

func TestSaveWidgetGeometry_SavesTheEditor(t *testing.T) {
	f, sceneID, widgetID := widgetFixture(t)

	f.p.widgetEdit.posX, f.p.widgetEdit.posY, f.p.widgetEdit.posZ = "100.5", "200", "3"
	f.p.widgetEdit.width, f.p.widgetEdit.height = "300", "120"
	f.p.widgetEdit.originX, f.p.widgetEdit.originY = "0", "1"
	f.p.widgetEdit.rotation = "45"
	f.p.saveWidgetGeometry(f.p.compoCtx)
	f.run()

	f.noToasts()
	w := f.widget(sceneID, widgetID)
	if w.Position != (apiv0.PositionResponse{X: 100.5, Y: 200, Z: 3}) || w.Size != (apiv0.SizeResponse{Width: 300, Height: 120}) ||
		w.Origin != (apiv0.OriginResponse{X: 0, Y: 1}) || w.Rotation.Degrees != 45 {
		t.Errorf("server widget: %+v", w)
	}
	f.knowsSceneVersion(sceneID)
}

func TestSaveWidgetGeometry_Refused_ShowsTheServersDetailAndPutsItsStateBack(t *testing.T) {
	f, sceneID, widgetID := widgetFixture(t)

	f.p.widgetEdit.originX = "2"
	f.p.saveWidgetGeometry(f.p.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != http.StatusBadRequest || got.Detail == "" || got.Method != http.MethodPut {
		t.Errorf("toast %+v: want the server's 400 with its detail", got)
	}
	if f.p.widgetEdit.originX != "0.500" {
		t.Errorf("editor origin X %q: want the server's back", f.p.widgetEdit.originX)
	}
	if w := f.widget(sceneID, widgetID); w.Origin.X != 0.5 {
		t.Errorf("server widget: %+v", w)
	}
}

func TestBindPortAndUnbindPort(t *testing.T) {
	f, sceneID, widgetID := widgetFixture(t, apiv0.InputPort{Name: "value", TypeHint: "integer"})
	tagID := createTag(t, f.api, "pump.speed", "integer")
	f.p.loadTags(f.p.compoCtx)
	f.run()

	f.p.bindPort(f.p.compoCtx, "value", tagID)
	f.run()

	f.noToasts()
	want := []apiv0.PortBinding{{PortName: "value", TagID: tagID}}
	if w := f.widget(sceneID, widgetID); len(w.PortBindings) != 1 || w.PortBindings[0] != want[0] {
		t.Errorf("bound: server bindings %+v, want %+v", w.PortBindings, want)
	}
	f.knowsSceneVersion(sceneID)

	f.p.unbindPort(f.p.compoCtx, "value")
	f.run()

	f.noToasts()
	if w := f.widget(sceneID, widgetID); len(w.PortBindings) != 0 {
		t.Errorf("unbound: server bindings %+v", w.PortBindings)
	}
	f.knowsSceneVersion(sceneID)
}

// drag moves the widget as a canvas drag would, then ends the drag.
func (f *fixture) drag(widgetID uuid.UUID, dx, dy float64) {
	f.p.draggingWidgetID = widgetID
	w, _ := f.p.findWidget(widgetID)
	f.p.dragStartGeometry = geometryOf(w)
	f.p.dragDidMove = dx != 0 || dy != 0
	for i := range f.p.widgets {
		if f.p.widgets[i].ID == widgetID {
			f.p.widgets[i].Position.X += dx
			f.p.widgets[i].Position.Y += dy
		}
	}
	f.p.finalizeAllDrags(f.p.compoCtx)
	f.run()
}

func TestDrag_SavesTheMovedWidget(t *testing.T) {
	f, sceneID, widgetID := widgetFixture(t)

	f.drag(widgetID, 5, 7)

	f.noToasts()
	if w := f.widget(sceneID, widgetID); w.Position.X != 15 || w.Position.Y != 27 {
		t.Errorf("server position: %+v", w.Position)
	}
	if f.p.widgetEdit.posX != "15.0" || f.p.widgetEdit.posY != "27.0" {
		t.Errorf("editor position: %q, %q", f.p.widgetEdit.posX, f.p.widgetEdit.posY)
	}
	f.knowsSceneVersion(sceneID)
}

func TestDrag_WithoutAMove_SavesNothing(t *testing.T) {
	f, sceneID, _ := widgetFixture(t)
	before := f.scene(sceneID).Version

	f.drag(f.p.selectedWidgetID, 0, 0)

	f.noToasts()
	if after := f.scene(sceneID).Version; after != before {
		t.Errorf("scene version %d → %d: a click saved the widget", before, after)
	}
}

// ── Tags ─────────────────────────────────────────────────────────────────────

func TestCreateTag_SelectsTheNewTag(t *testing.T) {
	f := startProject(t)

	f.p.creatingTag = true
	f.p.newTagName = "pump.on"
	f.p.newTagType = "boolean"
	f.p.createTag(f.p.compoCtx)
	f.run()

	f.noToasts()
	tags, err := f.api.ListTags(context.Background())
	if err != nil || len(tags) != 1 || tags[0].Name != "pump.on" || tags[0].Type != "boolean" || tags[0].Value != "false" {
		t.Fatalf("server tags: %+v, %v", tags, err)
	}
	if f.p.selectedTagID != tags[0].ID || f.p.creatingTag || len(f.p.tags.List()) != 1 {
		t.Errorf("selected %v, creating %v, %d tags", f.p.selectedTagID, f.p.creatingTag, len(f.p.tags.List()))
	}
}

func TestCreateTag_Refused_ShowsTheServersDetail(t *testing.T) {
	f := startProject(t)

	f.p.creatingTag = true
	f.p.newTagName = "pump.on"
	f.p.newTagType = "complex"
	f.p.createTag(f.p.compoCtx)
	f.run()

	got := f.oneToast()
	if got.Status != http.StatusBadRequest || got.Method != http.MethodPost || !strings.Contains(got.Detail, "complex") {
		t.Errorf("toast %+v: want the server's 400 naming the type", got)
	}
	if !f.p.creatingTag {
		t.Error("the create form was closed")
	}
}

func TestDeleteTag_RemovesItAndTheSelection(t *testing.T) {
	url, api := newServer(t)
	tagID := createTag(t, api, "pump.speed", "integer")
	f := mount(t, url, api)
	f.p.selectedTagID = tagID

	f.p.deleteTag(f.p.compoCtx)
	f.run()

	f.noToasts()
	if _, err := api.GetTag(context.Background(), tagID); err == nil {
		t.Error("the tag is still on the server")
	}
	if len(f.p.tags.List()) != 0 || f.p.selectedTagID != uuid.Nil {
		t.Errorf("tags %+v, selected %v: want none", f.p.tags.List(), f.p.selectedTagID)
	}
}

// ── No server ────────────────────────────────────────────────────────────────

func TestNoServer_ShowsNetworkErrors(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + l.Addr().String()
	l.Close()

	f := mount(t, url, nil)

	toasts := f.e.TakeToasts()
	if len(toasts) != 3 { // widget types, scenes, tags
		t.Fatalf("want a toast per list, got %+v", toasts)
	}
	for _, got := range toasts {
		if got.Title != "Network Error" || got.Status != 0 || !strings.Contains(got.Detail, l.Addr().String()) {
			t.Errorf("toast %+v: want a Network Error naming the server", got)
		}
	}

	f.p.createScene(f.p.compoCtx)
	f.run()
	if got := f.oneToast(); got.Title != "Network Error" {
		t.Errorf("create: toast %+v", got)
	}
}
