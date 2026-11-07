package player

import (
	"testing"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// recordingSource records what the Player asks of it and lets a test report
// what the stream does.
type recordingSource struct {
	opens  []int64
	events Events
	closes int
}

func (s *recordingSource) Open(_ app.Context, from int64, events Events) {
	s.opens = append(s.opens, from)
	s.events = events
}

func (s *recordingSource) Close() { s.closes++ }

// newTestPlayer is a Player with no component behind it: the Source is
// recorded and the timers are captured instead of run.
func newTestPlayer(source Source) (*Player, *[]func()) {
	var scheduled []func()
	p := &Player{
		source:     source,
		streamFrom: -1,
		retryDelay: initialRetryDelay,
	}
	p.after = func(_ time.Duration, f func()) { scheduled = append(scheduled, f) }
	p.updater = updateBatcher{
		interval: redrawInterval,
		after:    func(d time.Duration, f func()) { p.after(d, f) },
		request:  func() {},
	}
	return p, &scheduled
}

// TestPlayer_ReopensWhenTheStartingStateMoved: a stream opened from an older
// position than the starting state the Player now holds is reopened from the
// newer one, so the entries between the two are not replayed.
func TestPlayer_ReopensWhenTheStartingStateMoved(t *testing.T) {
	source := &recordingSource{}
	p, _ := newTestPlayer(source)
	p.open()
	if len(source.opens) != 1 || source.opens[0] != 0 {
		t.Fatalf("opens = %v, want one from 0", source.opens)
	}

	p.model.MergeSnapshot(apiv0.OperationStateResponse{Position: 7})
	source.events.Opened()
	if len(source.opens) != 2 || source.opens[1] != 7 {
		t.Fatalf("opens = %v, want the stream reopened from 7", source.opens)
	}

	source.events.Opened()
	if len(source.opens) != 2 {
		t.Errorf("an open at the current position reopened the stream: %v", source.opens)
	}
	if !p.Connected() {
		t.Error("the open stream did not mark the Player connected")
	}
}

// TestPlayer_LostConnection: a stream the browser retries only marks the
// Player disconnected; one it gave up on also schedules a full resync.
func TestPlayer_LostConnection(t *testing.T) {
	source := &recordingSource{}
	p, scheduled := newTestPlayer(source)
	p.open()

	source.events.Lost(false)
	if p.Connected() {
		t.Error("a lost stream left the Player connected")
	}
	if p.retryScheduled {
		t.Error("a native retry scheduled a resync")
	}

	source.events.Lost(true)
	if !p.retryScheduled {
		t.Error("a stream the browser gave up on scheduled no resync")
	}
	scheduledRetry := len(*scheduled)
	source.events.Lost(true)
	if len(*scheduled) != scheduledRetry {
		t.Errorf("repeated losses scheduled %d callbacks, want %d", len(*scheduled), scheduledRetry)
	}
}

// TestPlayer_Close: closing ends the source and ignores whatever a replaced
// stream still reports.
func TestPlayer_Close(t *testing.T) {
	source := &recordingSource{}
	p, _ := newTestPlayer(source)
	p.open()
	p.Close()

	if source.closes != 1 {
		t.Errorf("Close called the source %d times, want 1", source.closes)
	}
	// A callback of the closed stream is ignored.
	source.events.Lost(true)
	if p.retryScheduled {
		t.Error("a closed Player scheduled a resync")
	}
	p.Close()
	if source.closes != 1 {
		t.Errorf("the second Close called the source %d times, want 1", source.closes)
	}
}
