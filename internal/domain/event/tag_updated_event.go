package event

import (
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/tag"
)

type TagUpdatedEvent interface {
	TagEvent
}

type tagUpdatedEventImpl struct {
	tagEventImpl
}

var _ TagUpdatedEvent = (*tagUpdatedEventImpl)(nil)

func NewTagUpdatedEvent(aTagID id.ID[tag.Tag], opts ...EventOption) TagUpdatedEvent {
	ev := &tagUpdatedEventImpl{
		tagEventImpl: tagEventImpl{
			tagID: aTagID,
			eventImpl: eventImpl{
				eventType: EventTypeTagUpdated,
				timestamp: NewEventTimestamp(),
			},
		},
	}

	ev.applyOptions(opts...)

	return ev
}
