package eventlog

import (
	"slices"
	"strings"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestShouldNotifyError guards the fix for the CODE_REVIEW.md finding that
// EventSource errors were entirely invisible (dead onError field, never
// assigned). The native EventSource retries automatically and re-fires its
// error event on every failed attempt, so only the transition into
// "degraded" should notify — not every retry during a prolonged outage.
func TestShouldNotifyError(t *testing.T) {
	notify, degraded := shouldNotifyError(false)
	if !notify || !degraded {
		t.Errorf("first error should notify and mark degraded, got notify=%v degraded=%v", notify, degraded)
	}

	notify, degraded = shouldNotifyError(true)
	if notify || !degraded {
		t.Errorf("repeated error while already degraded should not notify again, got notify=%v degraded=%v", notify, degraded)
	}
}

// TestResyncsOnOpen: a connection that took over from one still carrying
// the server's events fires no EventResync (nothing of the server's was
// missed), only the resync of its project.
func TestResyncsOnOpen(t *testing.T) {
	both := []string{EventResync, EventProjectResync}
	for _, tt := range []struct {
		name       string
		resyncs    []string
		skipServer bool
		want       []string
	}{
		{"first open", both, false, both},
		{"switch while connected", both, true, []string{EventProjectResync}},
		{"switch to no project while connected", []string{EventResync}, true, []string{}},
		{"no project, first open", []string{EventResync}, false, []string{EventResync}},
	} {
		got := resyncsOnOpen(tt.resyncs, tt.skipServer)
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
	if !slices.Equal(both, []string{EventResync, EventProjectResync}) {
		t.Errorf("the connection's resyncs were changed: %v", both)
	}
}

// TestForwards: a connection superseded by that of another project delivers
// the server's events until it is closed, never its old project's content.
func TestForwards(t *testing.T) {
	for _, et := range apiv0.EventTypes() {
		if !forwards(false, et) {
			t.Errorf("%s from the current connection: not delivered", et)
		}
		content := strings.HasPrefix(et, "tag_") || strings.HasPrefix(et, "scene_") ||
			(strings.HasPrefix(et, "widget_") && !strings.HasPrefix(et, "widget_type_")) ||
			strings.HasPrefix(et, "library_pin_")
		if got := forwards(true, et); got == content {
			t.Errorf("%s from a superseded connection: delivered %t, want %t", et, got, !content)
		}
	}
}
