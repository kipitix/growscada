package eventlog

import "testing"

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
