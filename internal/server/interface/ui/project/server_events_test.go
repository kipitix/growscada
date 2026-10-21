package project

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// requestLog is a transport that records the requests sent through it.
type requestLog struct {
	mu   sync.Mutex
	reqs []string
}

func (l *requestLog) RoundTrip(r *http.Request) (*http.Response, error) {
	l.mu.Lock()
	l.reqs = append(l.reqs, r.Method+" "+r.URL.Path)
	l.mu.Unlock()
	return http.DefaultTransport.RoundTrip(r)
}

// take returns the requests sent so far, sorted, and forgets them.
func (l *requestLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	taken := l.reqs
	l.reqs = nil
	slices.Sort(taken)
	return taken
}

// mountLogged mounts a Project whose requests are recorded, and forgets the
// requests of its mounting.
func mountLogged(t *testing.T, url string, api *apiclient.Client) (*fixture, *requestLog) {
	t.Helper()
	log := &requestLog{}
	f := mountWith(t, url, api, &http.Client{Transport: log})
	log.take()
	return f, log
}

// event delivers ev to the Project as the event log does, and runs what it
// started.
func (f *fixture) event(ev apiv0.Event) {
	f.p.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, ev)
	f.run()
}

// selectScene selects the Scene as a click on its tab does.
func (f *fixture) selectScene(id uuid.UUID) {
	f.p.selectScene(f.p.compoCtx, id)
	f.run()
}

// twoScenes arranges two Scenes and a WidgetType, and mounts a Project, with
// its requests recorded, showing the first Scene. It returns the Scene shown
// and the other one.
func twoScenes(t *testing.T) (f *fixture, log *requestLog, shown, other, typeID uuid.UUID) {
	t.Helper()
	url, api := newServer(t)
	a := createScene(t, api, "A")
	b := createScene(t, api, "B")
	typeID = createWidgetType(t, api, "Lamp")
	f, log = mountLogged(t, url, api)
	shown = f.p.selectedSceneID
	if shown != a && shown != b {
		t.Fatalf("selected %v, want one of the Scenes", shown)
	}
	other = a
	if shown == a {
		other = b
	}
	return f, log, shown, other, typeID
}

// scenePath is the request of the Scene id of the Scene id.
func scenePath(id uuid.UUID) string { return "GET /api/v0/scenes/" + id.String() }

func TestWidgetEvent_SceneNotShown_RereadsThatSceneOnly(t *testing.T) {
	f, log, _, other, typeID := twoScenes(t)
	widgetID := createWidget(t, f.api, other, typeID, "w")

	f.event(apiv0.Event{Type: apiv0.EventWidgetCreated, ID: widgetID.String(), SceneID: other.String()})

	f.noToasts()
	if got, want := log.take(), []string{scenePath(other)}; !slices.Equal(got, want) {
		t.Errorf("requests: %v, want %v", got, want)
	}
	f.knowsSceneVersion(other)
	if sc, _ := uiutil.FindByID(f.p.scenes, other, uiutil.SceneID); sc.Version != f.scene(other).Version {
		t.Errorf("the listed Scene: %+v, want the server's", sc)
	}
}

func TestWidgetEvent_SceneShown_RereadsItsWidgetsOnly(t *testing.T) {
	f, log, shown, _, typeID := twoScenes(t)
	widgetID := createWidget(t, f.api, shown, typeID, "w")

	f.event(apiv0.Event{Type: apiv0.EventWidgetCreated, ID: widgetID.String(), SceneID: shown.String()})

	f.noToasts()
	// The Widgets carry the Scene's version.
	want := []string{scenePath(shown) + "/widgets"}
	if got := log.take(); !slices.Equal(got, want) {
		t.Errorf("requests: %v, want %v", got, want)
	}
	if len(f.p.widgets) != 1 || f.p.widgets[0].ID != widgetID {
		t.Errorf("widgets: %+v, want the new one", f.p.widgets)
	}
	f.knowsSceneVersion(shown)
}

func TestWidgetEvent_SceneNotListed_IsIgnored(t *testing.T) {
	f, log, _, _, _ := twoScenes(t)
	gone := uuid.New()

	f.event(apiv0.Event{Type: apiv0.EventWidgetDeleted, ID: uuid.NewString(), SceneID: gone.String()})

	f.noToasts() // a 404 is no error: the Scene was deleted meanwhile
	if got := log.take(); !slices.Equal(got, []string{scenePath(gone)}) {
		t.Errorf("requests: %v", got)
	}
	if len(f.p.scenes) != 2 {
		t.Errorf("scenes: %+v, want the two listed", f.p.scenes)
	}
}

func TestWidgetEvent_LastWidgetOfSceneNotShownDeleted_ItsPropertiesSaveWithoutConflict(t *testing.T) {
	f, _, _, other, typeID := twoScenes(t)
	widgetID := createWidget(t, f.api, other, typeID, "w")
	f.event(apiv0.Event{Type: apiv0.EventWidgetCreated, ID: widgetID.String(), SceneID: other.String()})
	if _, err := f.api.DeleteWidget(context.Background(), other, widgetID); err != nil {
		t.Fatalf("delete widget: %v", err)
	}

	f.event(apiv0.Event{Type: apiv0.EventWidgetDeleted, ID: widgetID.String(), SceneID: other.String()})
	f.selectScene(other) // no Widgets left to state the Scene's version
	f.p.sceneEdit.name = "Renamed"
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	f.noToasts()
	if sc := f.scene(other); sc.Name != "Renamed" {
		t.Errorf("server scene: %+v, want the edit", sc)
	}
}

func TestTagUpdated_AppliesTheTagWithoutRereadingTags(t *testing.T) {
	url, api := newServer(t)
	tagID := createTag(t, api, "speed", "integer")
	f, log := mountLogged(t, url, api)
	f.p.selectedTagID = tagID
	ctx := context.Background()
	tag, err := api.GetTag(ctx, tagID)
	if err != nil {
		t.Fatalf("get tag: %v", err)
	}
	if _, err := api.SetTagValue(ctx, tagID, "42", "good", tag.Version); err != nil {
		t.Fatalf("set tag value: %v", err)
	}
	if tag, err = api.GetTag(ctx, tagID); err != nil {
		t.Fatalf("get tag: %v", err)
	}

	f.event(apiv0.Event{Type: apiv0.EventTagUpdated, ID: tagID.String(), Tag: &tag})

	f.noToasts()
	if got := log.take(); len(got) != 0 {
		t.Errorf("requests: %v, want none", got)
	}
	if got, _ := f.p.findTag(tagID); got.Value != "42" || got.Version != tag.Version {
		t.Errorf("the Tag shown: %+v, want %+v", got, tag)
	}
	if list := f.p.tags.List(); len(list) != 1 || list[0].Value != "42" {
		t.Errorf("the Tag list: %+v, want the new value", list)
	}

	// Without its state, the Tags are reread.
	f.event(apiv0.Event{Type: apiv0.EventTagUpdated, ID: tagID.String()})
	if got := log.take(); !slices.Equal(got, []string{"GET /api/v0/tags"}) {
		t.Errorf("requests: %v, want the Tags reread", got)
	}
}

func TestSceneReread_RequestedBeforeASave_KeepsTheSave(t *testing.T) {
	f, _, shown, _, _ := twoScenes(t)
	stale, mark := f.scene(shown), f.p.markScenes() // a reread answered before the save

	f.p.startEditingScene(shown, "Renamed")
	f.p.commitSceneEdit(f.p.compoCtx)
	f.run()
	f.p.setScene(f.p.compoCtx, mark, stale)
	f.p.setScenes(f.p.compoCtx, mark, []apiv0.SceneResponse{stale, f.scene(f.p.scenes[1].ID)})
	f.run()

	f.noToasts()
	if sc, _ := f.p.findScene(shown); sc.Name != "Renamed" || f.p.sceneEdit.name != "Renamed" {
		t.Errorf("listed %q, editor %q: want the saved name", sc.Name, f.p.sceneEdit.name)
	}
	f.p.sceneEdit.width = "640"
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()
	f.noToasts()
	if sc := f.scene(shown); sc.Name != "Renamed" || sc.Width != 640 {
		t.Errorf("server scene: %+v, want the name kept and the new width", sc)
	}
}

func TestWidgetEvent_SceneNotListedYet_ItsVersionIsKept(t *testing.T) {
	f, _, _, _, typeID := twoScenes(t)
	added := createScene(t, f.api, "C")
	listed, mark := append(slices.Clone(f.p.scenes), f.scene(added)), f.p.markScenes() // a list read now, answered late
	widgetID := createWidget(t, f.api, added, typeID, "w")
	if _, err := f.api.DeleteWidget(context.Background(), added, widgetID); err != nil {
		t.Fatalf("delete widget: %v", err)
	}

	f.event(apiv0.Event{Type: apiv0.EventWidgetDeleted, ID: widgetID.String(), SceneID: added.String()})
	f.p.setScenes(f.p.compoCtx, mark, listed)
	f.run()
	f.selectScene(added)
	f.p.sceneEdit.name = "Renamed"
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	f.noToasts()
	if sc := f.scene(added); sc.Name != "Renamed" {
		t.Errorf("server scene: %+v, want the edit", sc)
	}
}

func TestWidgetEvent_DuringADrag_TheDragDoesNotOverwriteTheChange(t *testing.T) {
	f, sceneID, widgetID := widgetFixture(t)
	f.p.draggingWidgetID = widgetID
	theirs := f.widget(sceneID, widgetID)
	theirs.Name = "theirs"
	if _, err := f.api.UpdateWidget(context.Background(), sceneID, widgetID, widgetRequest(theirs, f.scene(sceneID).Version)); err != nil {
		t.Fatalf("update widget: %v", err)
	}

	f.event(apiv0.Event{Type: apiv0.EventWidgetUpdated, ID: widgetID.String(), SceneID: sceneID.String()})
	f.drag(widgetID, 5, 7) // saved with the Widgets shown

	if got := f.oneToast(); got.Status != http.StatusConflict {
		t.Errorf("toast %+v: want a conflict", got)
	}
	if w := f.widget(sceneID, widgetID); w.Name != "theirs" {
		t.Errorf("server widget: %+v, want the other change kept", w)
	}
}

func TestTagUpdated_OfATagDeletedMeanwhile_DoesNotShowIt(t *testing.T) {
	url, api := newServer(t)
	tagID := createTag(t, api, "speed", "integer")
	f, _ := mountLogged(t, url, api)
	tag, _ := f.p.findTag(tagID)
	f.p.selectedTagID = tagID
	f.p.deleteTag(f.p.compoCtx)
	f.run()

	tag.Value, tag.Version = "42", tag.Version+1 // committed before the delete, delivered after
	f.event(apiv0.Event{Type: apiv0.EventTagUpdated, ID: tagID.String(), Tag: &tag})

	if _, ok := f.p.findTag(tagID); ok || len(f.p.tags.List()) != 0 {
		t.Errorf("tags shown: %+v, want none", f.p.tags.List())
	}
}

func TestCreateTag_AListSentBefore_KeepsTheSelection(t *testing.T) {
	f := startProject(t)
	stale, mark, n := f.p.tags.List(), f.p.tags.Mark(), f.p.tagLists+1 // a list in flight
	f.p.tagLists = n

	f.p.newTagName, f.p.newTagType = "pump.on", "boolean"
	f.p.createTag(f.p.compoCtx)
	f.run()
	created := f.p.selectedTagID
	f.p.setTags(f.p.compoCtx, mark, n, stale)

	if f.p.selectedTagID != created || created == uuid.Nil {
		t.Errorf("selected %v, want the new Tag %v", f.p.selectedTagID, created)
	}
}

func TestWidgetEvents_InSeveralScenesNotShown_RereadTheListOnce(t *testing.T) {
	f, log, _, other, typeID := twoScenes(t)
	more := []uuid.UUID{other, createScene(t, f.api, "C"), createScene(t, f.api, "D")}
	f.p.loadScenes(f.p.compoCtx)
	f.run()
	log.take()
	for _, sceneID := range more { // a WidgetType changed: the Widgets of every Scene using it are reconciled
		widgetID := createWidget(t, f.api, sceneID, typeID, "w")
		f.p.compoCtx.NewActionWithValue(eventlog.ActionServerEvent,
			apiv0.Event{Type: apiv0.EventWidgetCreated, ID: widgetID.String(), SceneID: sceneID.String()})
	}
	f.run()

	f.noToasts()
	// The first Scene is reread by itself; the others, marked meanwhile, with the list.
	if got, want := log.take(), []string{"GET /api/v0/scenes", scenePath(more[0])}; !slices.Equal(got, want) {
		t.Errorf("requests: %v, want %v", got, want)
	}
	for _, sceneID := range more {
		f.knowsSceneVersion(sceneID)
	}
}
