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
	f.p.selectedSceneID = id
	uiutil.StoreID(f.p.compoCtx, sceneIDKey, id)
	f.p.clearWidgetSelection(f.p.compoCtx)
	f.p.syncSceneEditingFields()
	f.p.loadWidgets(f.p.compoCtx)
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

func TestWidgetEvent_SceneShown_RereadsTheSceneAndItsWidgets(t *testing.T) {
	f, log, shown, _, typeID := twoScenes(t)
	widgetID := createWidget(t, f.api, shown, typeID, "w")

	f.event(apiv0.Event{Type: apiv0.EventWidgetCreated, ID: widgetID.String(), SceneID: shown.String()})

	f.noToasts()
	want := []string{scenePath(shown), scenePath(shown) + "/widgets"}
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
	f.p.editingScenePropsName = "Renamed"
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
