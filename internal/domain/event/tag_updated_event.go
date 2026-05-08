package event

import (
	"github.com/kipitix/growscada/internal/domain/tag"
)

type TagUpdatedEvent interface {
	TagEvent
}

type tagUpdatedEventImpl struct {
	tagEventImpl
}

var _ TagUpdatedEvent = (*tagUpdatedEventImpl)(nil)

func NewTagUpdatedEvent(anTagID tag.TagID, opts ...EventOption) TagUpdatedEvent {
	ev := &tagUpdatedEventImpl{
		tagEventImpl: tagEventImpl{
			tagID: anTagID,
			eventImpl: eventImpl{
				eventType: EventTypeTagUpdated,
				timestamp: NewEventTimestamp(),
			},
		},
	}

	ev.applyOptions(opts...)

	return ev
}
