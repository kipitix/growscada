package event

import "fmt"

// EventType - enumeration for event type.
// The zero value is invalid (see docs/adr/0001-no-unknown-enum-sentinels.md).
// Value Object
type EventType struct {
	eventType int
}

var _ fmt.Stringer = EventType{}

// Sentinel values for EventType. Must not be reassigned.
var (
	EventTypeSystemReady        = EventType{eventType: 1}
	EventTypeTagCreated         = EventType{eventType: 2}
	EventTypeTagUpdated         = EventType{eventType: 3}
	EventTypeTagDeleted         = EventType{eventType: 4}
	EventTypeWidgetTypeCreated  = EventType{eventType: 5}
	EventTypeWidgetTypeUpdated  = EventType{eventType: 6}
	EventTypeWidgetTypeDeleted  = EventType{eventType: 7}
	EventTypeWidgetCreated      = EventType{eventType: 8}
	EventTypeWidgetUpdated      = EventType{eventType: 9}
	EventTypeWidgetDeleted      = EventType{eventType: 10}
	EventTypeSceneCreated       = EventType{eventType: 11}
	EventTypeSceneUpdated       = EventType{eventType: 12}
	EventTypeSceneDeleted       = EventType{eventType: 13}
	EventTypeClientConnected    = EventType{eventType: 14}
	EventTypeClientDisconnected = EventType{eventType: 15}
)

// AllEventTypes returns every known EventType.
func AllEventTypes() []EventType {
	return []EventType{
		EventTypeSystemReady,
		EventTypeTagCreated,
		EventTypeTagUpdated,
		EventTypeTagDeleted,
		EventTypeWidgetTypeCreated,
		EventTypeWidgetTypeUpdated,
		EventTypeWidgetTypeDeleted,
		EventTypeWidgetCreated,
		EventTypeWidgetUpdated,
		EventTypeWidgetDeleted,
		EventTypeSceneCreated,
		EventTypeSceneUpdated,
		EventTypeSceneDeleted,
		EventTypeClientConnected,
		EventTypeClientDisconnected,
	}
}

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
	case "scene_created":
		return EventTypeSceneCreated, nil
	case "scene_updated":
		return EventTypeSceneUpdated, nil
	case "scene_deleted":
		return EventTypeSceneDeleted, nil
	case "client_connected":
		return EventTypeClientConnected, nil
	case "client_disconnected":
		return EventTypeClientDisconnected, nil
	default:
		return EventType{}, fmt.Errorf("unknown event type: %q", s)
	}
}

// IsValid reports whether the event type is one of the known values (not the zero value).
func (et EventType) IsValid() bool {
	return et != EventType{}
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
	case EventTypeSceneCreated:
		return "scene_created"
	case EventTypeSceneUpdated:
		return "scene_updated"
	case EventTypeSceneDeleted:
		return "scene_deleted"
	case EventTypeClientConnected:
		return "client_connected"
	case EventTypeClientDisconnected:
		return "client_disconnected"
	default:
		return "invalid"
	}
}
