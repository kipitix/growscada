package project

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// The lists a mode reloads can be overtaken by the writes it makes: a list
// requested before a creation, a rename or a switch of Scene is older than
// what the mode already knows, and must not undo it (task 61). Each test
// takes a list — once loaded, a copy of it — then writes, then feeds the old
// list through the loader the way a late response arrives.

func TestCreateScene_AListSentBefore_KeepsTheSceneAndTheSelection(t *testing.T) {
	f := startProject(t)
	stale := slices.Clone(f.p.scenes) // a list in flight, before the Scene
	mark := f.p.draftVersions.Mark()

	f.p.createScene(f.p.compoCtx)
	f.run()
	created := f.p.selectedSceneID
	f.p.setScenes(f.p.compoCtx, mark, f.p.contentReplacements, stale)

	if len(f.p.scenes) != 1 || f.p.scenes[0].ID != created {
		t.Fatalf("scenes %+v: the new one %v is gone", f.p.scenes, created)
	}
	if f.p.selectedSceneID != created {
		t.Errorf("selected %v, want the new Scene %v", f.p.selectedSceneID, created)
	}
}

func TestCreateWidget_AListSentBefore_KeepsTheWidgetAndTheSelection(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	typeID := createWidgetType(t, api, "Lamp")
	f := mount(t, url, api)
	f.run()
	stale := slices.Clone(f.p.widgets) // a list in flight, before the Widget
	mark := f.p.draftVersions.Mark()

	f.p.createWidget(f.p.compoCtx, typeID, 10, 20)
	f.run()
	created := f.p.primaryWidgetID()
	f.p.setWidgets(f.p.compoCtx, mark, sceneID, stale)

	if len(f.p.widgets) != 1 || f.p.widgets[0].ID != created {
		t.Fatalf("widgets %+v: the new one %v is gone", f.p.widgets, created)
	}
	if f.p.primaryWidgetID() != created {
		t.Errorf("selected %v, want the new Widget %v", f.p.primaryWidgetID(), created)
	}
}

// A list of a Scene no longer shown cannot put its Widgets on the canvas: the
// selection moved while the response was on its way.
func TestSetWidgets_AListOfAnotherScene_IsDropped(t *testing.T) {
	url, api := newServer(t)
	first := createScene(t, api, "First")
	second := createScene(t, api, "Second")
	typeID := createWidgetType(t, api, "Lamp")
	createWidget(t, api, first, typeID, "theirs")
	f := mount(t, url, api)
	f.run()

	mark := f.p.draftVersions.Mark()
	stale := slices.Clone(f.p.widgets) // the Widgets of the first Scene
	stale[0].Name = "stale"
	f.p.selectScene(f.p.compoCtx, second)
	f.p.setWidgets(f.p.compoCtx, mark, first, stale)

	if f.p.widgets != nil || rendersText(f.p.canvasLayers(""), "stale") {
		t.Errorf("widgets %+v of the Scene left: want the canvas untouched", f.p.widgets)
	}
}

func TestScenesList_AnsweredAfterARename_DoesNotRollTheNameBack(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	f := mount(t, url, api)
	f.run()
	stale := slices.Clone(f.p.scenes) // a list in flight, before the rename
	mark := f.p.draftVersions.Mark()

	f.p.startEditingScene(sceneID, "Main")
	f.p.editingSceneName = "Renamed"
	f.p.commitSceneEdit(f.p.compoCtx)
	f.run()
	f.p.setScenes(f.p.compoCtx, mark, f.p.contentReplacements, stale)

	if name := f.p.scenes[0].Name; name != "Renamed" {
		t.Errorf("listed name %q: the stale list rolled it back", name)
	}
	if name := f.p.sceneEdit.name; name != "Renamed" {
		t.Errorf("editor name %q: the stale list rolled it back", name)
	}
	f.knowsVersion(sceneID)
}

// A switch of Scene empties the canvas at once: the Widgets of the Scene left
// are not the ones to draw, and the canvas says the others are on their way
// (task 61).
func TestSelectScene_ClearsTheCanvasUntilTheWidgetsAnswer(t *testing.T) {
	url, api := newServer(t)
	first := createScene(t, api, "First")
	second := createScene(t, api, "Second")
	typeID := createWidgetType(t, api, "Lamp")
	createWidget(t, api, first, typeID, "a")
	createWidget(t, api, second, typeID, "b")
	f := mount(t, url, api)
	f.run()

	f.p.selectScene(f.p.compoCtx, second)
	if f.p.widgets != nil {
		t.Fatalf("widgets %+v: the canvas still holds the Scene left", f.p.widgets)
	}
	if !f.p.widgetsLoading() {
		t.Error("the canvas does not say the Widgets are on their way")
	}
	if !rendersText(f.p.canvasLayers(""), "Loading widgets…") {
		t.Error("the canvas draws no loading notice")
	}

	f.run()
	if len(f.p.widgets) != 1 || f.p.widgets[0].SceneID != second {
		t.Fatalf("widgets %+v: want the ones of the Scene selected", f.p.widgets)
	}
	if f.p.widgetsLoading() {
		t.Error("the canvas still says it is loading")
	}
}

// A list stating the Version this client already knows is the server's own, not
// an older one: another read may have stated that Version first (the Draft's
// state on a resync, a reread Scene), and the list still replaces the copies
// shown — a change of another tab is not hidden by the read that observed it
// (task 61).
func TestScenesList_AtTheKnownVersion_TakesTheServersCopy(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	f := mount(t, url, api)
	f.run()
	renamed := f.scene(sceneID)
	renamed.Name = "Renamed"
	if _, err := api.UpdateScene(context.Background(), sceneID, apiv0.UpdateSceneRequest{
		Name: renamed.Name, Width: renamed.Width, Height: renamed.Height,
		BackgroundHTML: renamed.BackgroundHTML, Version: renamed.Version,
	}); err != nil {
		t.Fatalf("update scene: %v", err)
	}
	theirs := f.scene(sceneID) // the server's state, with the new Version
	f.p.observeDraftVersion(f.p.markDraft(), theirs.Version)
	mark := f.p.draftVersions.Mark() // a list requested after that read

	f.p.setScenes(f.p.compoCtx, mark, f.p.contentReplacements, []apiv0.SceneResponse{theirs})

	if got := f.p.scenes[0].Name; got != "Renamed" {
		t.Errorf("listed name %q: a list at the known Version kept the old copy", got)
	}
}

// The same for the Widgets: a list at the known Version is the server's own,
// whatever read observed that Version first (task 61).
func TestWidgetsList_AtTheKnownVersion_TakesTheServersCopy(t *testing.T) {
	f, sceneID, widgetID := widgetFixture(t)
	theirs := f.widget(sceneID, widgetID)
	theirs.Name = "theirs"
	if _, err := f.api.UpdateWidget(context.Background(), sceneID, widgetID, widgetRequest(theirs, f.scene(sceneID).Version)); err != nil {
		t.Fatalf("update widget: %v", err)
	}
	mine := f.widget(sceneID, widgetID)
	f.p.observeDraftVersion(f.p.markDraft(), mine.Version)
	mark := f.p.draftVersions.Mark()

	f.p.setWidgets(f.p.compoCtx, mark, sceneID, []apiv0.WidgetResponse{mine})

	if len(f.p.widgets) != 1 || f.p.widgets[0].Name != "theirs" {
		t.Errorf("widgets %+v: a list at the known Version kept the old copy", f.p.widgets)
	}
}

// rendersText reports whether one of the layers shows text.
func rendersText(layers []app.UI, text string) bool {
	for _, layer := range layers {
		if strings.Contains(app.HTMLString(layer), text) {
			return true
		}
	}
	return false
}

// failOnceScene breaks the first GET of a Scene, as a server hiccup does: the
// Scene stays stale and is reread by the next load (task 61).
type failOnceScene struct {
	inner  http.RoundTripper
	suffix string

	mu    sync.Mutex
	armed bool
	used  bool
}

func (f *failOnceScene) arm() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.armed, f.used = true, false
}

func (f *failOnceScene) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	fail := f.armed && !f.used && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, f.suffix)
	if fail {
		f.used = true
	}
	f.mu.Unlock()
	if !fail {
		return f.inner.RoundTrip(r)
	}
	body := `{"title":"Internal Server Error","status":500}`
	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Status:     "500 Internal Server Error",
		Header:     http.Header{"Content-Type": []string{"application/problem+json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}, nil
}

func TestLoadStaleScenes_AfterAFailedRead_IsRereadByTheNextLoad(t *testing.T) {
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	broken := &failOnceScene{
		inner:  http.DefaultTransport,
		suffix: "/scenes/" + sceneID.String(),
	}
	f := mountWith(t, url, api, &http.Client{Transport: broken})
	f.run()
	broken.arm()

	f.p.loadScene(f.p.compoCtx, sceneID)
	f.run()

	if got := f.oneToast(); got.Status != http.StatusInternalServerError {
		t.Errorf("toast %+v: want the failed read", got)
	}
	if _, ok := f.p.staleScenes[sceneID]; !ok {
		t.Fatalf("the failed Scene was forgotten: nothing will reread it")
	}

	f.p.loadStaleScenes(f.p.compoCtx)
	f.run()

	f.noToasts()
	if len(f.p.staleScenes) != 0 {
		t.Errorf("stale scenes %v: the reread left some", f.p.staleScenes)
	}
	if _, ok := f.p.findScene(sceneID); !ok {
		t.Errorf("the Scene is not listed")
	}
	f.knowsVersion(sceneID)
}
