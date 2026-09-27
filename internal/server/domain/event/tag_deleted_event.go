package event

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

type TagDeletedEvent interface {
	TagEvent
}

type tagDeletedEventImpl struct {
	tagEventImpl
}

var _ TagDeletedEvent = (*tagDeletedEventImpl)(nil)

func NewTagDeletedEvent(anID id.ID[tag.Tag], opts ...EventOption) TagDeletedEvent {
	ev := &tagDeletedEventImpl{
		tagEventImpl: tagEventImpl{
			tagID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeTagDeleted,
				timestamp: NewEventTimestamp(),
			},
		},
	}

	ev.applyOptions(opts...)

	return ev
}
