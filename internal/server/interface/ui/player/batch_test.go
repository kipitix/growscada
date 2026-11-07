package player

import (
	"testing"
	"time"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// TestUpdateBatcher_OneRedrawPerBurst: the first request schedules one redraw
// and the requests that arrive before it runs join it; after it, the next
// request schedules another.
func TestUpdateBatcher_OneRedrawPerBurst(t *testing.T) {
	var scheduled []func()
	var delays []time.Duration
	redraws := 0
	b := &updateBatcher{
		interval: 20 * time.Millisecond,
		after: func(d time.Duration, f func()) {
			delays = append(delays, d)
			scheduled = append(scheduled, f)
		},
		request: func() { redraws++ },
	}

	b.Request()
	b.Request()
	b.Request()
	if len(scheduled) != 1 {
		t.Fatalf("a burst scheduled %d redraws, want 1", len(scheduled))
	}
	if delays[0] != b.interval {
		t.Errorf("delay = %v, want %v", delays[0], b.interval)
	}
	if redraws != 0 {
		t.Errorf("the burst redrew %d times before the interval", redraws)
	}

	scheduled[0]()
	if redraws != 1 {
		t.Errorf("redraws = %d, want 1", redraws)
	}

	b.Request()
	if len(scheduled) != 2 {
		t.Errorf("a request after the redraw scheduled %d, want 2", len(scheduled))
	}
}

// TestPlayer_EntryBurstOneRedraw: entries are applied as they arrive, but a
// burst asks for one redraw.
func TestPlayer_EntryBurstOneRedraw(t *testing.T) {
	var scheduled []func()
	redraws := 0
	p := &Player{retryDelay: initialRetryDelay}
	p.after = func(_ time.Duration, f func()) { scheduled = append(scheduled, f) }
	p.updater = updateBatcher{
		interval: redrawInterval,
		after:    func(d time.Duration, f func()) { p.after(d, f) },
		request:  func() { p.onChange() },
	}
	p.onChange = func() { redraws++ }

	first, second := id("first"), id("second")
	p.model.SetRevision(revision(1, []projectv0.TagDefinition{
		{ID: first, Name: "first", Type: "integer"},
		{ID: second, Name: "second", Type: "integer"},
	}, nil, nil))

	p.apply(tagEntry(1, first, "1", "good"))
	p.apply(tagEntry(2, second, "2", "good"))
	if len(scheduled) != 1 {
		t.Fatalf("two entries scheduled %d redraws, want 1", len(scheduled))
	}
	// Both were applied though neither was drawn yet.
	if state, _ := p.model.TagState(second); state.Value != "2" {
		t.Errorf("the second entry was not applied: %+v", state)
	}

	scheduled[0]()
	if redraws != 1 {
		t.Errorf("redraws = %d, want 1", redraws)
	}
}
