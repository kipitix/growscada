package eventlog

import (
	"testing"

	"github.com/google/uuid"

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

func TestWithCheckedTag(t *testing.T) {
	tagID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	tag := &apiv0.TagResponse{ID: tagID, Value: "42", Version: 3}

	if got := withCheckedTag(apiv0.Event{Type: apiv0.EventTagUpdated, ID: tagID.String(), Tag: tag}); got.Tag != tag {
		t.Errorf("the Tag the event is about must be kept, got %+v", got)
	}
	if got := withCheckedTag(apiv0.Event{Type: apiv0.EventTagUpdated, ID: "00000000-0000-0000-0000-000000000002", Tag: tag}); got.Tag != nil {
		t.Errorf("another Tag must be dropped, got %+v", got.Tag)
	}
	if got := withCheckedTag(apiv0.Event{Type: apiv0.EventTagUpdated, ID: "", Tag: &apiv0.TagResponse{}}); got.Tag != nil {
		t.Errorf("a Tag without an ID must be dropped, got %+v", got.Tag)
	}
	ev := apiv0.Event{Type: apiv0.EventTagUpdated, Timestamp: "ts", ID: "x", Tag: tag}
	if got := withCheckedTag(ev); got.Type != ev.Type || got.Timestamp != ev.Timestamp || got.ID != ev.ID || got.Tag != nil {
		t.Errorf("only the Tag may be dropped, got %+v", got)
	}
}
