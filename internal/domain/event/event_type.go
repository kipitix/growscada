package event

import "fmt"

// EventType - enumeration for event type.
// Value Object
type EventType int

// String returns the string representation of the EventType.
var _ fmt.Stringer = EventType(0)

const (
	EventTypeUnknown EventType = iota
	EventTypeSystemReady
	EventTypeTagCreated
	EventTypeTagUpdated
	EventTypeTagDeleted
	EventTypeWidgetTypeCreated
	EventTypeWidgetTypeUpdated
	EventTypeWidgetTypeDeleted
	EventTypeWidgetCreated
	EventTypeWidgetUpdated
	EventTypeWidgetDeleted
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
	case "widget_type_created":
		return EventTypeWidgetTypeCreated, nil
	case "widget_type_updated":
		return EventTypeWidgetTypeUpdated, nil
	case "widget_type_deleted":
		return EventTypeWidgetTypeDeleted, nil
	case "widget_created":
		return EventTypeWidgetCreated, nil
	case "widget_updated":
		return EventTypeWidgetUpdated, nil
	case "widget_deleted":
		return EventTypeWidgetDeleted, nil
	default:
		return EventTypeUnknown, fmt.Errorf("unknown event type: %s", s)
	}
}

// String returns the string representation of the EventType.
// Implements the fmt.Stringer interface.
func (et EventType) String() string {
	switch et {
	case EventTypeUnknown:
		return "unknown"
	case EventTypeSystemReady:
		return "system_ready"
	case EventTypeTagCreated:
		return "tag_created"
	case EventTypeTagUpdated:
		return "tag_updated"
	case EventTypeTagDeleted:
		return "tag_deleted"
	case EventTypeWidgetTypeCreated:
		return "widget_type_created"
	case EventTypeWidgetTypeUpdated:
		return "widget_type_updated"
	case EventTypeWidgetTypeDeleted:
		return "widget_type_deleted"
	case EventTypeWidgetCreated:
		return "widget_created"
	case EventTypeWidgetUpdated:
		return "widget_updated"
	case EventTypeWidgetDeleted:
		return "widget_deleted"
	default:
		return "unknown"
	}
}
