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

type eventImpl struct {
	eventType EventType
	timestamp EventTimestamp
}

var _ Event = (*eventImpl)(nil)

// NO FABRIC METHOD

func (ev *eventImpl) applyOptions(opts ...EventOption) {
	for _, opt := range opts {
		opt(ev)
	}
}

type EventOption func(*eventImpl)

func WithTimestamp(timestamp EventTimestamp) EventOption {
	return func(event *eventImpl) {
		event.timestamp = timestamp
	}
}

func (ev eventImpl) Type() EventType {
	return ev.eventType
}

func (ev eventImpl) Timestamp() EventTimestamp {
	return ev.timestamp
}

func (ev eventImpl) String() string {
	return fmt.Sprintf("{ type=%s, timestamp=%s }", ev.eventType, ev.timestamp)
}
