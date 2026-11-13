package picker

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uitest"
)

// fakeSource is a Source with a per-user order and no server: the merge, the
// moves and the revert are what these tests are about (task 82).
type fakeSource struct {
	// mu guards the order: a test may read it while a write of it is in
	// flight, as the page itself does (task 82).
	mu       sync.Mutex
	items    []Item
	order    []uuid.UUID
	version  int
	setCalls int
	setIDs   []uuid.UUID
	setErr   error
	orderErr error
	// staleIDs, while staleReads is above zero, is what a read of the order
	// returns instead of the stored one: a read served before a write landed.
	staleIDs   []uuid.UUID
	staleReads int
	// orderCalls counts the reads of the order.
	orderCalls int
}

func (s *fakeSource) Title() string                        { return "Libraries" }
func (s *fakeSource) Singular() string                     { return "library" }
func (s *fakeSource) Plural() string                       { return "libraries" }
func (s *fakeSource) List(context.Context) ([]Item, error) { return s.items, nil }
func (s *fakeSource) Create(context.Context, string, string) (uuid.UUID, error) {
	return uuid.Nil, nil
}
func (s *fakeSource) Update(context.Context, uuid.UUID, int, string, string) error { return nil }
func (s *fakeSource) Delete(context.Context, uuid.UUID) error                      { return nil }
func (s *fakeSource) DeleteQuestion(name string) string                            { return "Delete " + name + "?" }

func (s *fakeSource) Order(context.Context) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orderCalls++
	if s.orderErr != nil {
		return Order{}, s.orderErr
	}
	if s.staleReads > 0 {
		s.staleReads--
		return Order{IDs: s.staleIDs, Version: s.version - 1}, nil
	}
	return Order{IDs: s.order, Version: s.version}, nil
}

func (s *fakeSource) SetOrder(_ context.Context, _ int, ids []uuid.UUID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCalls++
	s.setIDs = append([]uuid.UUID{}, ids...)
	if s.setErr != nil {
		return 0, s.setErr
	}
	s.version++
	s.order = append([]uuid.UUID{}, ids...)
	return s.version, nil
}

// serveStale makes the next reads of the order return ids, as a read served
// before a write landed does (task 82).
func (s *fakeSource) serveStale(ids []uuid.UUID, reads int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staleIDs, s.staleReads = ids, reads
}

func fakeItem(name string) Item { return Item{ID: uuid.New(), Name: name} }

// TestPicker_DisplayedMergesTheStoredOrder: the stored IDs come first, in
// their order, then the rest by name, and an ID the list does not have is
// ignored (task 81, ADR 0024).
func TestPicker_DisplayedMergesTheStoredOrder(t *testing.T) {
	alpha, bravo, charlie := fakeItem("Alpha"), fakeItem("Bravo"), fakeItem("Charlie")
	p := &Picker{
		items: []Item{charlie, alpha, bravo},
		order: Order{IDs: []uuid.UUID{bravo.ID, uuid.New()}},
	}

	shown := p.Displayed()
	if len(shown) != 3 || shown[0].Name != "Bravo" || shown[1].Name != "Alpha" || shown[2].Name != "Charlie" {
		t.Errorf("the merged order: %v", shown)
	}
}

// TestPicker_MoveStoresTheWholeOrder: a move sends every shown ID at once and
// keeps the Version the server stored (task 81, task 82).
func TestPicker_MoveStoresTheWholeOrder(t *testing.T) {
	alpha, bravo, charlie := fakeItem("Alpha"), fakeItem("Bravo"), fakeItem("Charlie")
	src := &fakeSource{items: []Item{charlie, alpha, bravo}}
	p := New(src, "test:picker", nil, nil)
	e := uitest.Load(t, mountOnHost{p})
	e.ConsumeAll()

	if shown := p.Displayed(); shown[0].Name != "Alpha" {
		t.Fatalf("the page lists %v, want them by name", shown)
	}

	p.move(p.compoCtx, 0, 2)
	e.ConsumeAll()

	if src.setCalls != 1 {
		t.Fatalf("stored the order %d times, want once", src.setCalls)
	}
	want := []uuid.UUID{bravo.ID, charlie.ID, alpha.ID}
	if len(src.setIDs) != len(want) {
		t.Fatalf("stored %v, want %v", src.setIDs, want)
	}
	for i := range want {
		if src.setIDs[i] != want[i] {
			t.Fatalf("stored %v, want %v", src.setIDs, want)
		}
	}
	if p.order.Version != 1 {
		t.Errorf("the version stored: %d, want 1", p.order.Version)
	}
	if shown := p.Displayed(); shown[0].Name != "Bravo" {
		t.Errorf("the page lists %v after the move", shown)
	}
}

// TestPicker_RefusedMovePutsTheOrderBack: a refused write — a conflict, say —
// restores the arrangement the page showed and says so (task 82).
func TestPicker_RefusedMovePutsTheOrderBack(t *testing.T) {
	alpha, bravo, charlie := fakeItem("Alpha"), fakeItem("Bravo"), fakeItem("Charlie")
	src := &fakeSource{items: []Item{charlie, alpha, bravo}}
	p := New(src, "test:picker", nil, nil)
	e := uitest.Load(t, mountOnHost{p})
	e.ConsumeAll()

	src.setErr = &apiclient.StatusError{Problem: apiv0.Problem{Status: http.StatusConflict}}
	p.move(p.compoCtx, 0, 2)
	e.ConsumeAll()

	if p.savingOrder {
		t.Error("a refused write left the page saving")
	}
	if shown := p.Displayed(); shown[0].Name != "Alpha" {
		t.Errorf("the page lists %v after the refusal, want the previous order", shown)
	}
	toasts := e.TakeToasts()
	if len(toasts) == 0 {
		t.Error("the refusal was not shown")
	}
}

// TestPicker_StaleOrderReadDoesNotUndoTheMove: a read of the order served
// while a move is being stored states the arrangement before it. Applied, it
// put the old arrangement back on screen and left the page out of step with
// the server until a reload; the page must show what the server kept (task 82).
func TestPicker_StaleOrderReadDoesNotUndoTheMove(t *testing.T) {
	alpha, bravo, charlie := fakeItem("Alpha"), fakeItem("Bravo"), fakeItem("Charlie")
	src := &fakeSource{items: []Item{charlie, alpha, bravo}}
	p := New(src, LibraryKey, nil, nil)
	e := uitest.Load(t, mountOnHost{p})
	e.ConsumeAll()

	p.move(p.compoCtx, 0, 2)
	src.serveStale([]uuid.UUID{alpha.ID, bravo.ID, charlie.ID}, 1)
	p.loadOrder(p.compoCtx)
	e.ConsumeAll()

	want := []uuid.UUID{bravo.ID, charlie.ID, alpha.ID}
	if got := p.order.IDs; !slices.Equal(got, want) {
		t.Errorf("the page shows the arrangement %v, want the stored %v", got, want)
	}
	if p.savingOrder {
		t.Error("the page is still saving")
	}
	if src.orderCalls < 3 {
		t.Errorf("the order was read %d times, want a read after the write", src.orderCalls)
	}
}

// TestPicker_KeyboardStepsAndMoves: the arrows step the selection, Alt with an
// arrow moves the selected entity (task 82).
func TestPicker_KeyboardStepsAndMoves(t *testing.T) {
	alpha, bravo, charlie := fakeItem("Alpha"), fakeItem("Bravo"), fakeItem("Charlie")
	src := &fakeSource{items: []Item{alpha, bravo, charlie}}
	p := New(src, "test:picker", nil, nil)
	e := uitest.Load(t, mountOnHost{p})
	e.ConsumeAll()

	p.selectItem(p.compoCtx, p.Displayed()[0])
	p.stepSelection(p.compoCtx, 1)
	if p.SelectedID() != bravo.ID {
		t.Fatalf("selected %v after one step, want Bravo", p.SelectedID())
	}
	p.stepSelection(p.compoCtx, 1)
	p.stepSelection(p.compoCtx, 1) // clamped at the last
	if p.SelectedID() != charlie.ID {
		t.Errorf("selected %v at the end, want Charlie", p.SelectedID())
	}

	p.selectItem(p.compoCtx, bravo)
	p.moveSelected(p.compoCtx, -1)
	e.ConsumeAll()
	if src.setCalls != 1 || src.setIDs[0] != bravo.ID {
		t.Errorf("the keyboard move stored %v", src.setIDs)
	}
}

// TestPicker_ProjectSourceHasOrder: the Projects carry a per-user order of
// their own (task 83), so their page arranges its rows as the Library page
// does (task 81, task 82).
func TestPicker_ProjectSourceHasOrder(t *testing.T) {
	src := NewProjectSource(apiclient.New("http://127.0.0.1:1", http.DefaultClient))
	if _, ok := src.(OrderSource); !ok {
		t.Fatal("the project source carries no order")
	}
	p := New(src, ProjectKey, nil, nil)
	if !p.sortable() {
		t.Error("the project page offers no drag")
	}
}

// TestPicker_LibrarySourceHasOrder: the library page arranges its rows; the
// source is what carries the order (task 81, task 82).
func TestPicker_LibrarySourceHasOrder(t *testing.T) {
	src := NewLibrarySource(apiclient.New("http://127.0.0.1:1", http.DefaultClient))
	if _, ok := src.(OrderSource); !ok {
		t.Fatal("the library source does not implement OrderSource")
	}
	if !New(src, LibraryKey, nil, nil).sortable() {
		t.Error("the library page offers no drag")
	}
}

// TestPicker_Reordered: the arithmetic of a drop moves one entity to a new
// position and refuses anything that moves nothing (task 82).
func TestPicker_Reordered(t *testing.T) {
	alpha, bravo, charlie := fakeItem("Alpha"), fakeItem("Bravo"), fakeItem("Charlie")
	shown := []Item{alpha, bravo, charlie}

	cases := []struct {
		from, to int
		want     []uuid.UUID
	}{
		{0, 2, []uuid.UUID{bravo.ID, charlie.ID, alpha.ID}},
		{2, 0, []uuid.UUID{charlie.ID, alpha.ID, bravo.ID}},
		{1, 1, nil},
		{-1, 0, nil},
		{0, 3, nil},
		{3, 0, nil},
	}
	for _, c := range cases {
		got := reordered(shown, c.from, c.to)
		if len(got) != len(c.want) {
			t.Errorf("reordered(%d, %d) = %v, want %v", c.from, c.to, got, c.want)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("reordered(%d, %d) = %v, want %v", c.from, c.to, got, c.want)
				break
			}
		}
	}
}

// TestPicker_MoveInvalidatesAnOlderOrderRead: a drop makes the response of an
// order read started before it stale, so the read cannot undo the drop and
// leave the next write stating an older Version (task 82).
func TestPicker_MoveInvalidatesAnOlderOrderRead(t *testing.T) {
	alpha, bravo, charlie := fakeItem("Alpha"), fakeItem("Bravo"), fakeItem("Charlie")
	src := &fakeSource{items: []Item{alpha, bravo, charlie}}
	p := New(src, "test:picker", nil, nil)
	e := uitest.Load(t, mountOnHost{p})
	e.ConsumeAll()

	p.orderSeq = 3 // as if a read were in flight
	p.move(p.compoCtx, 0, 2)
	if p.orderSeq != 4 {
		t.Errorf("the drop left the read sequence at %d, want it bumped", p.orderSeq)
	}
	e.ConsumeAll()
}

// TestPicker_OrderUnavailableShowsNoDrag: a server older than API 0.34 has no
// per-user settings: the page lists by name, offers no handle and does not
// toast on every load (task 82).
func TestPicker_OrderUnavailableShowsNoDrag(t *testing.T) {
	alpha, bravo := fakeItem("Alpha"), fakeItem("Bravo")
	src := &fakeSource{
		items:    []Item{bravo, alpha},
		orderErr: &apiclient.StatusError{Problem: apiv0.Problem{Status: http.StatusNotFound}},
	}
	p := New(src, "test:picker", nil, nil)
	e := uitest.Load(t, mountOnHost{p})
	e.ConsumeAll()

	if p.sortable() {
		t.Error("the page offers a drag without a server that keeps the order")
	}
	if html := app.HTMLString(p.Render()); strings.Contains(html, `data-picker-grip="1"`) {
		t.Errorf("the page shows a grip: %s", html)
	}
	if toasts := e.TakeToasts(); len(toasts) != 0 {
		t.Errorf("a missing setting toasted: %v", toasts)
	}
	if shown := p.Displayed(); shown[0].Name != "Alpha" {
		t.Errorf("the page lists %v, want them by name", shown)
	}
}

// TestPicker_CreateKeepsTheGripColumn: the draft row cannot be dragged, but the
// list must not shift when it opens: the column of the grip stays, empty, and
// the rows keep one height (task 82).
func TestPicker_CreateKeepsTheGripColumn(t *testing.T) {
	alpha, bravo := fakeItem("Alpha"), fakeItem("Bravo")
	p := New(&fakeSource{items: []Item{alpha, bravo}}, LibraryKey, nil, nil)
	e := uitest.Load(t, mountOnHost{p})
	e.ConsumeAll()

	before := app.HTMLString(p.Render())
	if got := strings.Count(before, `data-picker-grip="1"`); got != 2 {
		t.Fatalf("the list shows %d handles, want one per library", got)
	}

	p.startCreate(p.compoCtx)
	e.ConsumeAll()
	html := app.HTMLString(p.Render())

	if strings.Contains(html, `data-picker-grip="1"`) {
		t.Error("a draft row can be dragged from a handle")
	}
	if got, want := strings.Count(html, `data-picker-grip-space="1"`), 3; got != want {
		t.Errorf("the list keeps %d grip columns with a draft open, want %d", got, want)
	}
}
