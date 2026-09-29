package event

import (
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// TagUpdatedEvent is published when a Tag's value and Quality change. It
// carries the Tag's full new state, so subscribers need not read it back.
type TagUpdatedEvent interface {
	TagEvent
	Tag() tag.Tag
}

type tagUpdatedEventImpl struct {
	tagEventImpl
	tag tag.Tag
}

var _ TagUpdatedEvent = (*tagUpdatedEventImpl)(nil)

func NewTagUpdatedEvent(aTag tag.Tag, opts ...EventOption) TagUpdatedEvent {
	ev := &tagUpdatedEventImpl{
		tagEventImpl: tagEventImpl{
			tagID: aTag.ID(),
			eventImpl: eventImpl{
				eventType: EventTypeTagUpdated,
				timestamp: NewEventTimestamp(),
			},
		},
		tag: aTag,
	}

	ev.applyOptions(opts...)

	return ev
}

func (e tagUpdatedEventImpl) Tag() tag.Tag {
	return e.tag
}
