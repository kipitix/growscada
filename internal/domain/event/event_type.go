package event

import "fmt"

// EventType - enumeration for event type.
// Value Object
type EventType int

var _ fmt.Stringer = EventType(0)

const (
	EventTypeSystemReady EventType = iota
	EventTypeTagCreated
	EventTypeTagUpdated
	EventTypeTagDeleted
)

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

// func (et EventType) NewEvent() (Event, error) {
// 	switch et {
// 	case EventTypeSystemReady:
// 		return NewSystemReadyEvent(), nil
// 	case EventTypeTagCreated:
// 		return NewTagCreatedEvent(), nil
// 	case EventTypeTagUpdated:
// 		return NewTagUpdatedEvent(), nil
// 	case EventTypeTagDeleted:
// 		return NewTagDeletedEvent(), nil
// 	}
// 	return nil, fmt.Errorf("unknown event type: %s", et.String())
// }
