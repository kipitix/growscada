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
// A new value must also be added to eventTypeNames.
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

// eventTypeNames is the single source of truth for known event types:
// AllEventTypes, NewEventType and String are all derived from it.
// The order of entries is the order returned by AllEventTypes.
var eventTypeNames = []struct {
	eventType EventType
	name      string
}{
	{EventTypeSystemReady, "system_ready"},
	{EventTypeTagCreated, "tag_created"},
	{EventTypeTagUpdated, "tag_updated"},
	{EventTypeTagDeleted, "tag_deleted"},
	{EventTypeWidgetTypeCreated, "widget_type_created"},
	{EventTypeWidgetTypeUpdated, "widget_type_updated"},
	{EventTypeWidgetTypeDeleted, "widget_type_deleted"},
	{EventTypeWidgetCreated, "widget_created"},
	{EventTypeWidgetUpdated, "widget_updated"},
	{EventTypeWidgetDeleted, "widget_deleted"},
	{EventTypeSceneCreated, "scene_created"},
	{EventTypeSceneUpdated, "scene_updated"},
	{EventTypeSceneDeleted, "scene_deleted"},
	{EventTypeClientConnected, "client_connected"},
	{EventTypeClientDisconnected, "client_disconnected"},
}

// AllEventTypes returns every known EventType.
func AllEventTypes() []EventType {
	types := make([]EventType, len(eventTypeNames))
	for i, entry := range eventTypeNames {
		types[i] = entry.eventType
	}
	return types
}

// NewEventType creates a new EventType from a string representation.
// Returns an error if the string does not match any known event type.
func NewEventType(s string) (EventType, error) {
	for _, entry := range eventTypeNames {
		if entry.name == s {
			return entry.eventType, nil
		}
	}
	return EventType{}, fmt.Errorf("unknown event type: %q", s)
}

// IsValid reports whether the event type is one of the known values (not the zero value).
func (et EventType) IsValid() bool {
	return et != EventType{}
}

// String returns the string representation of the EventType.
// Implements the fmt.Stringer interface.
func (et EventType) String() string {
	for _, entry := range eventTypeNames {
		if entry.eventType == et {
			return entry.name
		}
	}
	return "invalid"
}
