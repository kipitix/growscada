package event

import (
	"fmt"
)

// Event - interface representing an event.
// Event is Value Object
type Event interface {
	Type() EventType
	Timestamp() EventTimestamp

	fmt.Stringer
}

// Base holds what every Event has: its type and timestamp. Concrete events,
// declared in the packages of their aggregates, embed it.
type Base struct {
	eventType EventType
	timestamp EventTimestamp
}

var _ Event = Base{}

// NewBase returns the Base of an event of the given type, stamped now unless
// an option says otherwise.
func NewBase(anEventType EventType, opts ...EventOption) Base {
	b := Base{eventType: anEventType, timestamp: NewEventTimestamp()}
	for _, opt := range opts {
		opt(&b)
	}
	return b
}

type EventOption func(*Base)

// WithTimestamp gives the event the timestamp it was recorded with, e.g. when
// it is read back from storage.
func WithTimestamp(timestamp EventTimestamp) EventOption {
	return func(event *Base) {
		event.timestamp = timestamp
	}
}

func (ev Base) Type() EventType {
	return ev.eventType
}

func (ev Base) Timestamp() EventTimestamp {
	return ev.timestamp
}

func (ev Base) String() string {
	return fmt.Sprintf("{ type=%s, timestamp=%s }", ev.eventType, ev.timestamp)
}

// Recorder is an aggregate that records the domain events of its changes
// until a repository saves it together with them. An aggregate a repository
// returns has recorded nothing.
type Recorder interface {
	// PendingEvents returns the events recorded since the aggregate was
	// created or read, oldest first.
	PendingEvents() []Event
}
