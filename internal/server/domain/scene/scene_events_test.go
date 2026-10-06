package scene

import (
	"slices"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// recorded describes a pending event as its type and the ID it names: the
// widget's for a WidgetEvent, the scene's for a SceneEvent.
type recorded struct {
	eventType event.EventType
	id        string
}

func pendingOf(t *testing.T, sc Scene) []recorded {
	t.Helper()
	var out []recorded
	for _, e := range sc.PendingEvents() {
		switch ev := e.(type) {
		case WidgetEvent:
			if ev.SceneID() != sc.ID() {
				t.Errorf("%s names scene %s, want %s", ev.Type(), ev.SceneID(), sc.ID())
			}
			out = append(out, recorded{ev.Type(), ev.WidgetID().String()})
		case SceneEvent:
			out = append(out, recorded{ev.Type(), ev.SceneID().String()})
		default:
			t.Fatalf("unexpected event %T", e)
		}
	}
	return out
}

func assertPending(t *testing.T, sc Scene, want ...recorded) {
	t.Helper()
	if got := pendingOf(t, sc); !slices.Equal(got, want) {
		t.Errorf("pending events %v, want %v", got, want)
	}
}

func TestCreateScene_RecordsCreatedEventAtInitialVersion(t *testing.T) {
	name, _ := NewSceneName("boiler_room")
	size, _ := NewSceneSize(800, 600)
	sc := CreateScene(id.NewID[Scene](), name, size, NewBackgroundHTML(""))

	if sc.Version() != version.Initial[Scene]() {
		t.Errorf("expected initial version, got %s", sc.Version())
	}
	if len(sc.Widgets()) != 0 {
		t.Errorf("expected no widgets, got %d", len(sc.Widgets()))
	}
	assertPending(t, sc, recorded{event.EventTypeSceneCreated, sc.ID().String()})
}

func TestReconstituteScene_RecordsNothing(t *testing.T) {
	assertPending(t, makeStoredScene(t, makeWidgetOf(t, makeWidgetType(t), "w1")))
}

func TestScene_Changes_RecordTheirEventsInOrder(t *testing.T) {
	wt := makeWidgetType(t, "value")
	kept := makeWidgetOf(t, wt, "kept")
	sc := makeStoredScene(t, kept)
	added := makeWidgetOf(t, wt, "added", "value")

	afterUpdate, err := sc.Update(sc.Version(), sc.Name(), sc.Size(), NewBackgroundHTML("<svg/>"))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	afterAdd, err := afterUpdate.AddWidget(sc.Version(), added, wt)
	if err != nil {
		t.Fatalf("AddWidget: %v", err)
	}
	afterUpdateWidget, err := afterAdd.UpdateWidget(sc.Version(), added, wt)
	if err != nil {
		t.Fatalf("UpdateWidget: %v", err)
	}
	afterRemove, _, err := afterUpdateWidget.RemoveWidget(kept.ID())
	if err != nil {
		t.Fatalf("RemoveWidget: %v", err)
	}

	assertPending(t, afterRemove,
		recorded{event.EventTypeSceneUpdated, sc.ID().String()},
		recorded{event.EventTypeWidgetCreated, added.ID().String()},
		recorded{event.EventTypeWidgetUpdated, added.ID().String()},
		recorded{event.EventTypeWidgetDeleted, kept.ID().String()},
	)
	assertPending(t, sc)
	assertPending(t, afterUpdate, recorded{event.EventTypeSceneUpdated, sc.ID().String()})
}

func TestScene_RejectedChange_RecordsNothing(t *testing.T) {
	sc := makeStoredScene(t)

	if _, err := sc.Update(staleVersion(sc), sc.Name(), sc.Size(), sc.BackgroundHTML()); err == nil {
		t.Fatal("expected conflict")
	}
	if _, _, err := sc.RemoveWidget(id.NewID[Widget]()); err == nil {
		t.Fatal("expected ErrWidgetNotFound")
	}
	assertPending(t, sc)
}

func TestSceneReconcileWith_RecordsWidgetUpdatedForEachChangedWidget(t *testing.T) {
	before := makeWidgetType(t, "value", "alarm")
	changed1 := makeWidgetOf(t, before, "w1", "value", "alarm")
	untouched := makeWidgetOf(t, before, "w2", "value")
	changed2 := makeWidgetOf(t, before, "w3", "alarm")
	sc := makeStoredScene(t, changed1, untouched, changed2)
	after := makeWidgetTypeWithID(t, before.ID(), "value")

	reconciled, ok := sc.ReconcileWith(after)

	if !ok {
		t.Fatal("expected a change")
	}
	assertPending(t, reconciled,
		recorded{event.EventTypeWidgetUpdated, changed1.ID().String()},
		recorded{event.EventTypeWidgetUpdated, changed2.ID().String()},
	)
}

func TestSceneDelete_RecordsWidgetDeletedForEachWidgetThenSceneDeleted(t *testing.T) {
	wt := makeWidgetType(t)
	w1 := makeWidgetOf(t, wt, "w1")
	w2 := makeWidgetOf(t, wt, "w2")
	sc := makeStoredScene(t, w1, w2)

	deleted := sc.Delete()

	assertPending(t, deleted,
		recorded{event.EventTypeWidgetDeleted, w1.ID().String()},
		recorded{event.EventTypeWidgetDeleted, w2.ID().String()},
		recorded{event.EventTypeSceneDeleted, sc.ID().String()},
	)
	if deleted.Version() != sc.Version() || !slices.Equal(deleted.Widgets(), sc.Widgets()) {
		t.Error("Delete must keep the scene's state for the CAS")
	}
	assertPending(t, sc)
}

func TestWidgetEvents_AreNoSceneEvents(t *testing.T) {
	widgetID, sceneID := id.NewID[Widget](), id.NewID[Scene]()
	for _, e := range []event.Event{
		NewWidgetCreatedEvent(widgetID, sceneID),
		NewWidgetUpdatedEvent(widgetID, sceneID),
		NewWidgetDeletedEvent(widgetID, sceneID),
	} {
		if _, ok := e.(SceneEvent); ok {
			t.Errorf("%s is a SceneEvent: a type switch would take it for an event of the scene", e.Type())
		}
	}
	for _, e := range []event.Event{
		NewSceneCreatedEvent(sceneID),
		NewSceneUpdatedEvent(sceneID),
		NewSceneDeletedEvent(sceneID),
	} {
		if _, ok := e.(SceneEvent); !ok {
			t.Errorf("%s is no SceneEvent", e.Type())
		}
	}
}
