package event

import "fmt"

// EventType - enumeration for event type.
// Value Object
type EventType int

// String returns the string representation of the EventType.
var _ fmt.Stringer = EventType(0)

const (
	EventTypeSystemReady EventType = iota
	EventTypeTagCreated
	EventTypeTagUpdated
	EventTypeTagDeleted
)

// NewEventType creates a new EventType from a string representation.
// Returns an error if the string does not match any known event type.
func NewEventType(s string) (EventType, error) {
	switch s {
	case "system_ready":
		return EventTypeSystemReady, nil
	case "tag_created":
		return EventTypeTagCreated, nil
	case "tag_updated":
		return EventTypeTagUpdated, nil
	case "tag_deleted":
		return EventTypeTagDeleted, nil
	default:
		return 0, fmt.Errorf("unknown event type: %s", s)
	}
}

// String returns the string representation of the EventType.
// Implements the fmt.Stringer interface.
func (et EventType) String() string {
	switch et {
	case EventTypeSystemReady:
		return "system_ready"
	case EventTypeTagCreated:
		return "tag_created"
	case EventTypeTagUpdated:
		return "tag_updated"
	case EventTypeTagDeleted:
		return "tag_deleted"
	default:
		return "unknown"
	}
}