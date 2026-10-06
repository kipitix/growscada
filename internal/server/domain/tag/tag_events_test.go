package tag

import (
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

func makeCreatedTag(t *testing.T) Tag {
	t.Helper()
	name, _ := NewTagName("pressure")
	value, _ := TagTypeInteger.NewTagValue(int64(42))
	aTag, err := CreateTag(id.NewID[Tag](), name, TagTypeInteger, value, TagQualityGood)
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	return aTag
}

func TestCreateTag_RecordsCreatedEventAtInitialVersion(t *testing.T) {
	aTag := makeCreatedTag(t)

	if aTag.Version() != version.Initial[Tag]() {
		t.Errorf("expected initial version, got %s", aTag.Version())
	}
	events := aTag.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(events))
	}
	created, ok := events[0].(TagCreatedEvent)
	if !ok {
		t.Fatalf("expected TagCreatedEvent, got %T", events[0])
	}
	if created.Type() != event.EventTypeTagCreated || created.TagID() != aTag.ID() {
		t.Errorf("unexpected event %s for tag %s", created, created.TagID())
	}
}

func TestReconstituteTag_RecordsNothing(t *testing.T) {
	if got := makeTestTag(t).PendingEvents(); len(got) != 0 {
		t.Errorf("expected no pending events, got %v", got)
	}
}

func TestTag_SetValue_RecordsUpdatedEventWithSavedState(t *testing.T) {
	aTag := makeTestTag(t)
	if err := aTag.SetValue(int64(7), TagQualityBad); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	events := aTag.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(events))
	}
	updated, ok := events[0].(TagUpdatedEvent)
	if !ok {
		t.Fatalf("expected TagUpdatedEvent, got %T", events[0])
	}
	state := updated.Tag()
	if state.Value().String() != "7" || state.Quality() != TagQualityBad {
		t.Errorf("event carries %s, want value 7 quality bad", state)
	}
	if state.Version() != aTag.Version().Next() {
		t.Errorf("event carries version %s, want the one Save stores: %s", state.Version(), aTag.Version().Next())
	}
	if len(state.PendingEvents()) != 0 {
		t.Error("the state an event carries records no events")
	}
}

func TestTag_SetValue_Invalid_RecordsNothing(t *testing.T) {
	aTag := makeTestTag(t)
	if err := aTag.SetValue(3.14, TagQualityGood); err == nil {
		t.Fatal("expected error")
	}
	if got := aTag.PendingEvents(); len(got) != 0 {
		t.Errorf("expected no pending events, got %v", got)
	}
}

func TestTag_Delete_RecordsDeletedEvent(t *testing.T) {
	aTag := makeTestTag(t)
	aTag.Delete()

	events := aTag.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(events))
	}
	deleted, ok := events[0].(TagDeletedEvent)
	if !ok || deleted.TagID() != aTag.ID() {
		t.Errorf("expected TagDeletedEvent for %s, got %v", aTag.ID(), events[0])
	}
}

func TestTag_PendingEvents_ReturnsCopy(t *testing.T) {
	aTag := makeCreatedTag(t)
	events := aTag.PendingEvents()
	events[0] = nil
	if aTag.PendingEvents()[0] == nil {
		t.Error("changing the returned slice changed the tag")
	}
}

func TestNewTagCreatedEvent_WithTimestamp_SetsTimestamp(t *testing.T) {
	fixed := event.NewEventTimestamp(event.EventTimestampWithTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)))
	e := NewTagCreatedEvent(id.NewID[Tag](), event.WithTimestamp(fixed))

	if !e.Timestamp().Time().Equal(fixed.Time()) {
		t.Errorf("expected %v, got %v", fixed.Time(), e.Timestamp().Time())
	}
}

func TestNewTagDeletedEvent_TypeAndID(t *testing.T) {
	tagID := id.NewID[Tag]()
	e := NewTagDeletedEvent(tagID)
	if e.Type() != event.EventTypeTagDeleted || e.TagID() != tagID {
		t.Errorf("unexpected event %s for tag %s", e, e.TagID())
	}
	if e.Timestamp().Time().IsZero() {
		t.Error("expected non-zero timestamp")
	}
}
