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

func TestParseEvent_ReadsTolerantly(t *testing.T) {
	ev, ok := parseEvent(`{"type":"tag_updated","timestamp":"2026-10-13T10:00:00Z",
		"id":"00000000-0000-0000-0000-000000000001","origin":"device",
		"tag":{"id":"00000000-0000-0000-0000-000000000001","name":"speed","type":"integer",
		"value":"42","quality":"good","version":3,"unit":"rpm"}}`)
	if !ok || ev.Type != "tag_updated" || ev.Tag == nil || ev.Tag.Value != "42" || ev.Tag.Version != 3 {
		t.Fatalf("got %+v, %v", ev, ok)
	}

	if ev, ok := parseEvent(`{"type":"scene_created","id":"x"}`); !ok || ev.Tag != nil {
		t.Errorf("an event without a Tag: got %+v, %v", ev, ok)
	}
	for _, bad := range []string{``, `not json`, `{}`, `[]`} {
		if _, ok := parseEvent(bad); ok {
			t.Errorf("parseEvent(%q) should fail", bad)
		}
	}
}
