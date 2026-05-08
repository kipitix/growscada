package event

import (
	"github.com/kipitix/growscada/internal/domain/tag"
)

type TagCreatedEvent interface {
	TagEvent
}

type tagCreatedEventImpl struct {
	tagEventImpl
}

var _ TagCreatedEvent = (*tagCreatedEventImpl)(nil)

func NewTagCreatedEvent(anID tag.TagID, opts ...EventOption) TagCreatedEvent {
	ev := &tagCreatedEventImpl{
		tagEventImpl: tagEventImpl{
			tagID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeTagCreated,
				timestamp: NewEventTimestamp(),
			},
		},
	}

	ev.applyOptions(opts...)

	return ev
}
