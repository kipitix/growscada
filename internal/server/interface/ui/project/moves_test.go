package project

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// moveCounter records the method and path of every request the Project
// sends, so a test can assert how many PATCHes a gesture cost, and in what
// order the writes went out.
type moveCounter struct {
	mu      sync.Mutex
	entries []string
}

func (c *moveCounter) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.entries = append(c.entries, r.Method+" "+r.URL.Path)
	c.mu.Unlock()
	return http.DefaultTransport.RoundTrip(r)
}

func (c *moveCounter) patches() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, e := range c.entries {
		if len(e) >= 5 && e[:5] == "PATCH" {
			n++
		}
	}
	return n
}

func (c *moveCounter) writes() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.entries))
	for _, e := range c.entries {
		switch {
		case len(e) >= 6 && e[:6] == "DELETE":
			out = append(out, e)
		case len(e) >= 5 && e[:5] == "PATCH":
			out = append(out, e)
		case len(e) >= 3 && e[:3] == "PUT":
			out = append(out, e)
		case len(e) >= 4 && e[:4] == "POST":
			out = append(out, e)
		}
	}
	return out
}

// moveFixture is selectionFixture with a client whose write requests are
// counted.
func moveFixture(t *testing.T, n int) (*fixture, uuid.UUID, []uuid.UUID, *moveCounter) {
	t.Helper()
	url, api := newServer(t)
	sceneID := createScene(t, api, "Main")
	typeID := createWidgetType(t, api, "Gauge")
	ids := make([]uuid.UUID, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, createWidget(t, api, sceneID, typeID, "gauge"+string(rune('a'+i))))
	}
	spreadWidgets(t, api, sceneID, ids)
	counter := &moveCounter{}
	f := mountWith(t, url, api, &http.Client{Transport: counter})
	f.run()
	f.p.clearWidgetSelection(f.p.compoCtx)
	return f, sceneID, ids, counter
}

func TestArrowStep_IsOnePixelOrTenWithShift(t *testing.T) {
	cases := []struct {
		key   string
		shift bool
		dx    float64
		dy    float64
		ok    bool
	}{
		{"ArrowLeft", false, -1, 0, true},
		{"ArrowRight", true, 10, 0, true},
		{"ArrowUp", true, 0, -10, true},
		{"ArrowDown", false, 0, 1, true},
		{"a", false, 0, 0, false},
	}
	for _, c := range cases {
		dx, dy, ok := arrowStep(c.key, c.shift)
		if ok != c.ok || dx != c.dx || dy != c.dy {
			t.Errorf("arrowStep(%q, %v) = (%v, %v, %v), want (%v, %v, %v)", c.key, c.shift, dx, dy, ok, c.dx, c.dy, c.ok)
		}
	}
}

func TestArrowMove_OnePatchMovesTheWholeSelection(t *testing.T) {
	f, sceneID, ids, counter := moveFixture(t, 2)
	f.p.selectAllWidgets(f.p.compoCtx)

	f.p.beginArrowMove(f.p.compoCtx, 1, 0)
	f.p.beginArrowMove(f.p.compoCtx, 0, 2)
	f.p.flushArrowMoves(f.p.compoCtx) // the arrow keyup
	f.run()

	f.noToasts()
	if got := counter.patches(); got != 1 {
		t.Errorf("%d PATCHes for one series: want one", got)
	}
	for i, id := range ids {
		w := f.widget(sceneID, id)
		wantX := 11 + 200*float64(i)
		if w.Position.X != wantX || w.Position.Y != 22 {
			t.Errorf("widget %d position %+v: want (%v, 22)", i, w.Position, wantX)
		}
	}
	f.knowsVersion(sceneID)
}

func TestArrowMove_BackToTheStartSendsNothing(t *testing.T) {
	f, sceneID, ids, counter := moveFixture(t, 1)
	f.p.selectOnly(f.p.compoCtx, ids[0])
	before := f.scene(sceneID).Version

	f.p.beginArrowMove(f.p.compoCtx, 3, 0)
	f.p.beginArrowMove(f.p.compoCtx, -3, 0)
	f.p.flushArrowMoves(f.p.compoCtx)
	f.run()

	f.noToasts()
	if got := counter.patches(); got != 0 {
		t.Errorf("%d PATCHes: a series back at the start saved nothing", got)
	}
	if after := f.scene(sceneID).Version; after != before {
		t.Errorf("scene version %d → %d", before, after)
	}
}

func TestArrowMove_ReadOnlyMovesNothingAndAsksNothing(t *testing.T) {
	f, sceneID, ids, counter := moveFixture(t, 1)
	f.p.selectOnly(f.p.compoCtx, ids[0])
	f.p.editLock = apiv0.EditLockResponse{} // no lock: read-only

	f.p.beginArrowMove(f.p.compoCtx, 5, 0)
	f.p.flushArrowMoves(f.p.compoCtx)
	f.run()

	if got := counter.patches(); got != 0 {
		t.Errorf("%d PATCHes in read-only", got)
	}
	if w := f.widget(sceneID, ids[0]); w.Position.X != 10 {
		t.Errorf("read-only moved the widget to %v", w.Position)
	}
	f.noToasts() // and no toast per key press
}

// A write that follows an arrow series is sent only after the series' PATCH
// was answered: both state the Draft's Version.
func TestArrowMove_AnotherWriteWaitsForThePatch(t *testing.T) {
	f, sceneID, ids, counter := moveFixture(t, 2)
	f.p.selectAllWidgets(f.p.compoCtx)

	f.p.beginArrowMove(f.p.compoCtx, 5, 0)
	f.p.deleteWidget(f.p.compoCtx, ids[0])
	f.run()

	f.noToasts()
	writes := counter.writes()
	if len(writes) != 2 || writes[0][:5] != "PATCH" || writes[1][:6] != "DELETE" {
		t.Fatalf("writes %v: want the move's PATCH, then the delete", writes)
	}
	if _, err := f.api.GetWidget(context.Background(), sceneID, ids[0]); err == nil {
		t.Error("the delete did not happen")
	}
	if w := f.widget(sceneID, ids[1]); w.Position.X != 215 {
		t.Errorf("the moved widget is at %v, want 215", w.Position)
	}
	f.knowsVersion(sceneID)
}

// Two writes that wait for the same move PATCH both go out after it, in the
// order they came: the drain does not put a waiting write back into the queue,
// where it would wait for itself.
func TestArrowMove_TwoWritesWaitForThePatch(t *testing.T) {
	f, sceneID, ids, counter := moveFixture(t, 2)
	f.p.selectOnly(f.p.compoCtx, ids[0])

	f.p.beginArrowMove(f.p.compoCtx, 5, 0)
	f.p.deleteWidget(f.p.compoCtx, ids[1])
	// A write the panel asks for without a port or a Tag: it sends nothing,
	// but it waits for the move like every other write of the Draft.
	f.p.bindPort(f.p.compoCtx, "", uuid.Nil)
	f.run()

	f.noToasts()
	writes := counter.writes()
	if len(writes) != 2 || writes[0][:5] != "PATCH" || writes[1][:6] != "DELETE" {
		t.Fatalf("writes %v: want the move's PATCH, then the delete", writes)
	}
	if _, err := f.api.GetWidget(context.Background(), sceneID, ids[1]); err == nil {
		t.Error("the delete that waited for the move did not happen")
	}
	if w := f.widget(sceneID, ids[0]); w.Position.X != 15 {
		t.Errorf("the moved widget is at %v, want 15", w.Position)
	}
	f.knowsVersion(sceneID)
}

func TestDrag_GroupMovesOnOnePatch(t *testing.T) {
	f, sceneID, ids, counter := moveFixture(t, 3)
	f.p.selectAllWidgets(f.p.compoCtx)

	f.drag(ids[0], 5, 7)

	f.noToasts()
	if got := counter.patches(); got != 1 {
		t.Errorf("%d PATCHes for a group drag: want one", got)
	}
	for i, id := range ids {
		w := f.widget(sceneID, id)
		if w.Position.X != 15+float64(i)*200 || w.Position.Y != 27 {
			t.Errorf("widget %d position %+v: all must ride the same delta", i, w.Position)
		}
	}
	f.knowsVersion(sceneID)
}

func TestDrag_SingleWidgetIsOnePatch(t *testing.T) {
	f, sceneID, ids, counter := moveFixture(t, 2)
	f.p.selectOnly(f.p.compoCtx, ids[0])

	f.drag(ids[0], 3, 4)

	f.noToasts()
	if got := counter.patches(); got != 1 {
		t.Errorf("%d PATCHes for a single drag: want one", got)
	}
	if w := f.widget(sceneID, ids[0]); w.Position.X != 13 || w.Position.Y != 24 {
		t.Errorf("position %+v: want (13, 24)", w.Position)
	}
	if w := f.widget(sceneID, ids[1]); w.Position.X != 210 {
		t.Errorf("the unselected widget moved to %v", w.Position)
	}
}

func TestDrag_GroupBackAtTheStartSendsNothing(t *testing.T) {
	f, sceneID, ids, counter := moveFixture(t, 2)
	f.p.selectAllWidgets(f.p.compoCtx)
	before := f.scene(sceneID).Version

	f.drag(ids[0], 0, 0)

	f.noToasts()
	if got := counter.patches(); got != 0 {
		t.Errorf("%d PATCHes: a drag without a move is a click", got)
	}
	if after := f.scene(sceneID).Version; after != before {
		t.Errorf("scene version %d → %d", before, after)
	}
	if got := f.selection(); len(got) != 1 || got[0] != ids[0] {
		t.Errorf("the click did not make %v the only selected: %v", ids[0], got)
	}
}

// movedFrom skips a Widget that already holds the requested position: the
// group's PATCH is those that moved.
func TestMovedFrom_SkipsUnchanged(t *testing.T) {
	a := apiv0.WidgetResponse{ID: uuid.New(), Position: apiv0.PositionResponse{X: 10, Y: 20}}
	b := apiv0.WidgetResponse{ID: uuid.New(), Position: apiv0.PositionResponse{X: 99, Y: 20}}
	p := &Project{widgets: []apiv0.WidgetResponse{a, b}}
	base := map[uuid.UUID]apiv0.PositionResponse{a.ID: {X: 10, Y: 20}, b.ID: {X: 10, Y: 20}}

	moves := p.movedFrom(base, []uuid.UUID{a.ID, b.ID})
	if len(moves) != 1 || moves[0].WidgetID != b.ID || moves[0].X != 99 {
		t.Errorf("moves: %+v", moves)
	}
}
