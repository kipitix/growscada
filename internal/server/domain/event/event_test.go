package event_test

import (
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
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

func TestNewEventType_UnrecognisedString_ReturnsErrorAndZeroValue(t *testing.T) {
	for _, input := range []string{"unknown_type", "unknown", ""} {
		t.Run(input, func(t *testing.T) {
			got, err := event.NewEventType(input)
			if err == nil {
				t.Errorf("expected error for %q, got nil", input)
			}
			if got.IsValid() {
				t.Errorf("expected zero value, got %v", got)
			}
		})
	}
}

func TestEventType_IsValid(t *testing.T) {
	if !event.EventTypeTagCreated.IsValid() {
		t.Error("expected EventTypeTagCreated to be valid")
	}
	if (event.EventType{}).IsValid() {
		t.Error("expected zero value to be invalid")
	}
}

func TestEventType_String_ZeroValueIsInvalid(t *testing.T) {
	if got := (event.EventType{}).String(); got != "invalid" {
		t.Errorf("expected %q, got %q", "invalid", got)
	}
}

func TestEventType_String_RoundTripsForAllEventTypes(t *testing.T) {
	for _, et := range event.AllEventTypes() {
		t.Run(et.String(), func(t *testing.T) {
			if et.String() == "invalid" {
				t.Fatalf("expected a name, got %q", et.String())
			}
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

func TestAllEventTypes_ValuesAndNamesAreUnique(t *testing.T) {
	seenTypes := make(map[event.EventType]bool)
	seenNames := make(map[string]bool)
	for _, et := range event.AllEventTypes() {
		if seenTypes[et] {
			t.Errorf("duplicate event type %v", et)
		}
		if seenNames[et.String()] {
			t.Errorf("duplicate event type name %q", et.String())
		}
		seenTypes[et] = true
		seenNames[et.String()] = true
	}
}

// --- AllEventTypes ---

func TestAllEventTypes_ExcludesZeroValue(t *testing.T) {
	for _, et := range event.AllEventTypes() {
		if !et.IsValid() {
			t.Error("expected AllEventTypes to not include the zero value")
		}
	}
}

func TestAllEventTypes_IncludesClientLifecycleTypes(t *testing.T) {
	types := event.AllEventTypes()
	hasConnected, hasDisconnected := false, false
	for _, et := range types {
		if et == event.EventTypeClientConnected {
			hasConnected = true
		}
		if et == event.EventTypeClientDisconnected {
			hasDisconnected = true
		}
	}
	if !hasConnected || !hasDisconnected {
		t.Errorf("expected AllEventTypes to include client connected/disconnected, got %v", types)
	}
}

// --- Client lifecycle type names ---

func TestEventType_String_ClientLifecycle(t *testing.T) {
	if got := event.EventTypeClientConnected.String(); got != "client_connected" {
		t.Errorf("expected %q, got %q", "client_connected", got)
	}
	if got := event.EventTypeClientDisconnected.String(); got != "client_disconnected" {
		t.Errorf("expected %q, got %q", "client_disconnected", got)
	}
}

func TestNewEventType_ClientLifecycleStrings_RoundTrip(t *testing.T) {
	for _, s := range []string{"client_connected", "client_disconnected"} {
		got, err := event.NewEventType(s)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", s, err)
		}
		if got.String() != s {
			t.Errorf("round-trip failed: expected %q, got %q", s, got.String())
		}
	}
}
