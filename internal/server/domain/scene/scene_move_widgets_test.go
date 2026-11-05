package scene

import (
	"errors"
	"slices"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// ── MoveWidgets ─────────────────────────────────────────────────────────────

func TestSceneMoveWidgets_MovesOnlyTheGivenWidgets(t *testing.T) {
	wt := makeWidgetType(t)
	w1 := makeWidgetOf(t, wt, "w1")
	w2 := makeWidgetOf(t, wt, "w2")
	w3 := makeWidgetOf(t, wt, "w3")
	sc := makeStoredScene(t, w1, w2, w3)

	after, moved, err := sc.MoveWidgets([]WidgetMove{
		NewWidgetMove(w1.ID(), 10, 20),
		NewWidgetMove(w3.ID(), -5, 7.5),
	})

	if err != nil {
		t.Fatalf("MoveWidgets: %v", err)
	}
	if got, want := ids(moved), ids([]Widget{w1, w3}); !slices.Equal(got, want) {
		t.Errorf("moved: expected %v, got %v", want, got)
	}
	if got, want := positionOf(t, after, w1), NewPosition(10, 20, 0); got != want {
		t.Errorf("w1 position: expected %s, got %s", want, got)
	}
	if got, want := positionOf(t, after, w3), NewPosition(-5, 7.5, 0); got != want {
		t.Errorf("w3 position: expected %s, got %s", want, got)
	}
	if got, want := positionOf(t, after, w2), w2.Position(); got != want {
		t.Errorf("w2 moved: expected %s, got %s", want, got)
	}
	assertUnchanged(t, sc, []Widget{w1, w2, w3})
}

func TestSceneMoveWidgets_KeepsEverythingButThePosition(t *testing.T) {
	wt := makeWidgetType(t, "value")
	name, _ := NewWidgetName("gauge")
	origin, _ := NewOrigin(0.25, 0.75)
	bound := NewWidget(id.NewID[Widget](), name, NewPosition(1, 2, 3), library.DefaultSize(),
		origin, NewRotation(45), wt.ID(), []string{"a", "b"},
		[]PortBinding{NewPortBinding(mustPortName(t, "value"), id.NewID[tag.Tag]())})
	sc := makeStoredScene(t, bound)

	after, _, err := sc.MoveWidgets([]WidgetMove{NewWidgetMove(bound.ID(), 30, 40)})

	if err != nil {
		t.Fatalf("MoveWidgets: %v", err)
	}
	moved := mustFind(t, after, bound.ID())
	if got, want := moved.Position(), NewPosition(30, 40, 3); got != want {
		t.Errorf("position: expected %s, got %s", want, got)
	}
	if moved.Name() != bound.Name() || moved.Size() != bound.Size() || moved.Origin() != bound.Origin() ||
		moved.Rotation() != bound.Rotation() || moved.TypeID() != bound.TypeID() ||
		!slices.Equal(moved.Labels(), bound.Labels()) ||
		!slices.Equal(moved.PortBindings(), bound.PortBindings()) {
		t.Errorf("the move changed more than the position: %s", moved)
	}
}

func TestSceneMoveWidgets_RecordsOneEventPerMovedWidgetInOrder(t *testing.T) {
	wt := makeWidgetType(t)
	w1 := makeWidgetOf(t, wt, "w1")
	w2 := makeWidgetOf(t, wt, "w2")
	sc := makeStoredScene(t, w1, w2)

	after, _, err := sc.MoveWidgets([]WidgetMove{
		NewWidgetMove(w2.ID(), 5, 5),
		NewWidgetMove(w1.ID(), 6, 6),
	})

	if err != nil {
		t.Fatalf("MoveWidgets: %v", err)
	}
	assertPending(t, after,
		recorded{event.EventTypeWidgetUpdated, w2.ID().String()},
		recorded{event.EventTypeWidgetUpdated, w1.ID().String()},
	)
}

// A widget already at the position it is moved to is not touched: no new
// widget, no event and no entry in the answer.
func TestSceneMoveWidgets_SkipsWidgetsThatWouldNotMove(t *testing.T) {
	wt := makeWidgetType(t)
	staying := makeWidgetOf(t, wt, "staying")
	moving := makeWidgetOf(t, wt, "moving")
	sc := makeStoredScene(t, staying, moving)

	after, moved, err := sc.MoveWidgets([]WidgetMove{
		NewWidgetMove(staying.ID(), staying.Position().X(), staying.Position().Y()),
		NewWidgetMove(moving.ID(), 9, 9),
	})

	if err != nil {
		t.Fatalf("MoveWidgets: %v", err)
	}
	if got, want := ids(moved), ids([]Widget{moving}); !slices.Equal(got, want) {
		t.Errorf("moved: expected %v, got %v", want, got)
	}
	if got := mustFind(t, after, staying.ID()); got != staying {
		t.Errorf("the widget that stays was rebuilt: %s", got)
	}
	assertPending(t, after, recorded{event.EventTypeWidgetUpdated, moving.ID().String()})
}

func TestSceneMoveWidgets_NothingToMove(t *testing.T) {
	wt := makeWidgetType(t)
	w1 := makeWidgetOf(t, wt, "w1")
	sc := makeStoredScene(t, w1)

	after, moved, err := sc.MoveWidgets([]WidgetMove{
		NewWidgetMove(w1.ID(), w1.Position().X(), w1.Position().Y()),
	})

	if !errors.Is(err, ErrNothingToMove) {
		t.Fatalf("expected ErrNothingToMove, got %v", err)
	}
	if after != nil || moved != nil {
		t.Errorf("a refused move returned a scene or widgets: %v, %v", after, moved)
	}
	assertUnchanged(t, sc, []Widget{w1})
	if pending := sc.PendingEvents(); len(pending) != 0 {
		t.Errorf("a refused move recorded %d events", len(pending))
	}
}

func TestSceneMoveWidgets_Rejected(t *testing.T) {
	wt := makeWidgetType(t)
	w1 := makeWidgetOf(t, wt, "w1")
	w2 := makeWidgetOf(t, wt, "w2")
	sc := makeStoredScene(t, w1, w2)

	tests := map[string]struct {
		moves []WidgetMove
		want  error
	}{
		"unknown widget": {
			[]WidgetMove{NewWidgetMove(w1.ID(), 1, 1), NewWidgetMove(id.NewID[Widget](), 2, 2)},
			ErrWidgetNotFound,
		},
		"the same widget twice": {
			[]WidgetMove{NewWidgetMove(w1.ID(), 1, 1), NewWidgetMove(w2.ID(), 2, 2), NewWidgetMove(w1.ID(), 3, 3)},
			ErrWidgetMovedTwice,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := sc.MoveWidgets(tt.moves)
			if !errors.Is(err, tt.want) {
				t.Errorf("expected %v, got %v", tt.want, err)
			}
			assertUnchanged(t, sc, []Widget{w1, w2})
		})
	}
}

// positionOf is the position of the widget with widgetID in sc.
func positionOf(t *testing.T, sc Scene, withID Widget) Position {
	t.Helper()
	return mustFind(t, sc, withID.ID()).Position()
}

// mustFind is FindWidget failing the test.
func mustFind(t *testing.T, sc Scene, widgetID id.ID[Widget]) Widget {
	t.Helper()
	found, err := sc.FindWidget(widgetID)
	if err != nil {
		t.Fatalf("FindWidget(%s): %v", widgetID, err)
	}
	return found
}
