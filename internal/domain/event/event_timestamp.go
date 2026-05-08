package event

import (
	"fmt"
	"time"
)

// EventTimestamp - time.Time wrapper.
// EventTimestamp is Value Object.
type EventTimestamp time.Time

// Interface implementation.
var _ fmt.Stringer = EventTimestamp(time.Time{})

// NewEventTimestamp - creates new EventTimestamp.
// Accepts options.
// If no options provided, uses current time.
func NewEventTimestamp(opts ...EventTimestampOption) EventTimestamp {
	// Create current time
	et := EventTimestamp(time.Now())

	// Apply options
	for _, opt := range opts {
		opt(&et)
	}

	// If no options provided, use default value
	return et
}

// EventTimestampOption - option for EventTimestamp.
type EventTimestampOption func(*EventTimestamp)

// EventTimestampWIthTime allows specifying an existing time for EventTimestamp creation.
// Used when an EventTimestamp needs to be created from an existing time.
func EventTimestampWIthTime(t time.Time) EventTimestampOption {
	return func(et *EventTimestamp) {
		*et = EventTimestamp(t)
	}
}

// ParseEventTimestamp - parses EventTimestamp from string.
// Returns error if parsing fails.
func ParseEventTimestamp(s string) (EventTimestamp, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return EventTimestamp{}, err
	}
	return EventTimestamp(t), nil
}

// MustParseEventTimestamp - parses EventTimestamp from string.
// Panics if parsing fails.
func MustParseEventTimestamp(s string) EventTimestamp {
	t, err := ParseEventTimestamp(s)
	if err != nil {
		panic(err)
	}
	return t
}

// Get time.
func (et EventTimestamp) Time() time.Time {
	return time.Time(et)
}

// String representation.
func (et EventTimestamp) String() string {
	return time.Time(et).String()
}
