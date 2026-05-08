package event_test

import (
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/tag"
)

// --- EventTimestamp ---

func TestNewEventTimestamp_NoOptions_IsCloseToNow(t *testing.T) {
	before := time.Now()
	ts := event.NewEventTimestamp()
	after := time.Now()

	if ts.Time().Before(before) || ts.Time().After(after) {
		t.Errorf("timestamp %v is not between %v and %v", ts.Time(), before, after)
	}
}

func TestNewEventTimestamp_WithTime_UsesProvidedTime(t *testing.T) {
	fixed := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	ts := event.NewEventTimestamp(event.EventTimestampWithTime(fixed))

	if !ts.Time().Equal(fixed) {
		t.Errorf("expected %v, got %v", fixed, ts.Time())
	}
}

func TestEventTimestamp_String_IsNonEmpty(t *testing.T) {
	ts := event.NewEventTimestamp()
	if ts.String() == "" {
		t.Error("expected non-empty string representation")
	}
}

func TestParseEventTimestamp_ValidRFC3339_ReturnsTimestamp(t *testing.T) {
	input := "2025-06-01T10:00:00Z"
	ts, err := event.ParseEventTimestamp(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	if !ts.Time().Equal(expected) {
		t.Errorf("expected %v, got %v", expected, ts.Time())
	}
}

func TestParseEventTimestamp_InvalidString_ReturnsError(t *testing.T) {
	_, err := event.ParseEventTimestamp("not-a-date")
	if err == nil {
		t.Error("expected error for invalid timestamp string, got nil")
	}
}

func TestMustParseEventTimestamp_ValidString_ReturnsTimestamp(t *testing.T) {
	input := "2025-06-01T10:00:00Z"
	ts := event.MustParseEventTimestamp(input)

	expected := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	if !ts.Time().Equal(expected) {
		t.Errorf("expected %v, got %v", expected, ts.Time())
	}
}

func TestMustParseEventTimestamp_InvalidString_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid timestamp string, got none")
		}
	}()
	event.MustParseEventTimestamp("not-a-date")
}

// --- EventType ---

func TestNewEventType_ValidStrings_ReturnsCorrectType(t *testing.T) {
	cases := []struct {
		input    string
		expected event.EventType
	}{
		{"system_ready", event.EventTypeSystemReady},
		{"tag_created", event.EventTypeTagCreated},
		{"tag_updated", event.EventTypeTagUpdated},
		{"tag_deleted", event.EventTypeTagDeleted},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got, err := event.NewEventType(c.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.expected {
				t.Errorf("expected %v, got %v", c.expected, got)
			}
		})
	}
}

func TestNewEventType_UnknownString_ReturnsErrorAndUnknownType(t *testing.T) {
	got, err := event.NewEventType("unknown_type")
	if err == nil {
		t.Error("expected error for unknown event type, got nil")
	}
	if got != event.EventTypeUnknown {
		t.Errorf("expected EventTypeUnknown, got %v", got)
	}
}

func TestEventType_String_Unknown(t *testing.T) {
	if got := event.EventTypeUnknown.String(); got != "unknown" {
		t.Errorf("expected %q, got %q", "unknown", got)
	}
}

func TestEventType_String_RoundTrips(t *testing.T) {
	types := []event.EventType{
		event.EventTypeSystemReady,
		event.EventTypeTagCreated,
		event.EventTypeTagUpdated,
		event.EventTypeTagDeleted,
	}

	for _, et := range types {
		t.Run(et.String(), func(t *testing.T) {
			parsed, err := event.NewEventType(et.String())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed != et {
				t.Errorf("round-trip failed: expected %v, got %v", et, parsed)
			}
		})
	}
}

// --- EventBus ---

func TestEventBus_Publish_CallsSubscribedHandler(t *testing.T) {
	bus := event.NewEventBus()
	var received []event.Event
	bus.Subscribe(event.EventTypeTagCreated, func(e event.Event) {
		received = append(received, e)
	})

	e := event.NewTagCreatedEvent(tag.NewTagID())
	bus.Publish(e)

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type() != event.EventTypeTagCreated {
		t.Errorf("expected %s, got %s", event.EventTypeTagCreated, received[0].Type())
	}
}

func TestEventBus_Publish_DoesNotCallHandlerForDifferentType(t *testing.T) {
	bus := event.NewEventBus()
	var received []event.Event
	bus.Subscribe(event.EventTypeTagDeleted, func(e event.Event) {
		received = append(received, e)
	})

	bus.Publish(event.NewTagCreatedEvent(tag.NewTagID()))

	if len(received) != 0 {
		t.Errorf("expected no events, got %d", len(received))
	}
}

func TestEventBus_Publish_CallsAllSubscribersForSameType(t *testing.T) {
	bus := event.NewEventBus()
	count := 0
	bus.Subscribe(event.EventTypeTagCreated, func(e event.Event) { count++ })
	bus.Subscribe(event.EventTypeTagCreated, func(e event.Event) { count++ })

	bus.Publish(event.NewTagCreatedEvent(tag.NewTagID()))

	if count != 2 {
		t.Errorf("expected 2 handler calls, got %d", count)
	}
}

func TestEventBus_Publish_NoSubscribers_DoesNotPanic(t *testing.T) {
	bus := event.NewEventBus()
	bus.Publish(event.NewTagCreatedEvent(tag.NewTagID()))
}

// --- SystemReadyEvent ---

func TestNewSystemReadyEvent_Type_IsSystemReady(t *testing.T) {
	e := event.NewSystemReadyEvent()
	if e.Type() != event.EventTypeSystemReady {
		t.Errorf("expected %s, got %s", event.EventTypeSystemReady, e.Type())
	}
}

func TestNewSystemReadyEvent_Timestamp_IsNonZero(t *testing.T) {
	e := event.NewSystemReadyEvent()
	if e.Timestamp().Time().IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewSystemReadyEvent_String_IsNonEmpty(t *testing.T) {
	e := event.NewSystemReadyEvent()
	if e.String() == "" {
		t.Error("expected non-empty string representation")
	}
}

func TestNewSystemReadyEvent_WithTimestamp_SetsTimestamp(t *testing.T) {
	fixed := event.NewEventTimestamp(event.EventTimestampWithTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)))
	e := event.NewSystemReadyEvent(event.WithTimestamp(fixed))

	if !e.Timestamp().Time().Equal(fixed.Time()) {
		t.Errorf("expected %v, got %v", fixed.Time(), e.Timestamp().Time())
	}
}

// --- TagCreatedEvent ---

func TestNewTagCreatedEvent_Type_IsTagCreated(t *testing.T) {
	e := event.NewTagCreatedEvent(tag.NewTagID())
	if e.Type() != event.EventTypeTagCreated {
		t.Errorf("expected %s, got %s", event.EventTypeTagCreated, e.Type())
	}
}

func TestNewTagCreatedEvent_TagID_MatchesProvided(t *testing.T) {
	id := tag.NewTagID()
	e := event.NewTagCreatedEvent(id)

	if e.TagID() != id {
		t.Errorf("expected TagID %v, got %v", id, e.TagID())
	}
}

func TestNewTagCreatedEvent_Timestamp_IsNonZero(t *testing.T) {
	e := event.NewTagCreatedEvent(tag.NewTagID())
	if e.Timestamp().Time().IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewTagCreatedEvent_String_IsNonEmpty(t *testing.T) {
	e := event.NewTagCreatedEvent(tag.NewTagID())
	if e.String() == "" {
		t.Error("expected non-empty string representation")
	}
}

func TestNewTagCreatedEvent_WithTimestamp_SetsTimestamp(t *testing.T) {
	fixed := event.NewEventTimestamp(event.EventTimestampWithTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)))
	e := event.NewTagCreatedEvent(tag.NewTagID(), event.WithTimestamp(fixed))

	if !e.Timestamp().Time().Equal(fixed.Time()) {
		t.Errorf("expected %v, got %v", fixed.Time(), e.Timestamp().Time())
	}
}

// --- TagUpdatedEvent ---

func TestNewTagUpdatedEvent_Type_IsTagUpdated(t *testing.T) {
	e := event.NewTagUpdatedEvent(tag.NewTagID())
	if e.Type() != event.EventTypeTagUpdated {
		t.Errorf("expected %s, got %s", event.EventTypeTagUpdated, e.Type())
	}
}

func TestNewTagUpdatedEvent_TagID_MatchesProvided(t *testing.T) {
	id := tag.NewTagID()
	e := event.NewTagUpdatedEvent(id)

	if e.TagID() != id {
		t.Errorf("expected TagID %v, got %v", id, e.TagID())
	}
}

func TestNewTagUpdatedEvent_Timestamp_IsNonZero(t *testing.T) {
	e := event.NewTagUpdatedEvent(tag.NewTagID())
	if e.Timestamp().Time().IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewTagUpdatedEvent_String_IsNonEmpty(t *testing.T) {
	e := event.NewTagUpdatedEvent(tag.NewTagID())
	if e.String() == "" {
		t.Error("expected non-empty string representation")
	}
}

// --- TagDeletedEvent ---

func TestNewTagDeletedEvent_Type_IsTagDeleted(t *testing.T) {
	e := event.NewTagDeletedEvent(tag.NewTagID())
	if e.Type() != event.EventTypeTagDeleted {
		t.Errorf("expected %s, got %s", event.EventTypeTagDeleted, e.Type())
	}
}

func TestNewTagDeletedEvent_TagID_MatchesProvided(t *testing.T) {
	id := tag.NewTagID()
	e := event.NewTagDeletedEvent(id)

	if e.TagID() != id {
		t.Errorf("expected TagID %v, got %v", id, e.TagID())
	}
}

func TestNewTagDeletedEvent_Timestamp_IsNonZero(t *testing.T) {
	e := event.NewTagDeletedEvent(tag.NewTagID())
	if e.Timestamp().Time().IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewTagDeletedEvent_String_IsNonEmpty(t *testing.T) {
	e := event.NewTagDeletedEvent(tag.NewTagID())
	if e.String() == "" {
		t.Error("expected non-empty string representation")
	}
}
