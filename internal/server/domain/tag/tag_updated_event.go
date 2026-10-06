package tag

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
)

// TagUpdatedEvent is recorded when a Tag's value and Quality change. It
// carries the Tag's full new state, Version included, so subscribers need not
// read it back.
type TagUpdatedEvent interface {
	TagEvent
	Tag() Tag
}

type tagUpdatedEventImpl struct {
	tagEventImpl
	tag Tag
}

var _ TagUpdatedEvent = (*tagUpdatedEventImpl)(nil)

// NewTagUpdatedEvent returns the event carrying aTag as the Tag's new state.
func NewTagUpdatedEvent(aTag Tag, opts ...event.EventOption) TagUpdatedEvent {
	return &tagUpdatedEventImpl{
		tagEventImpl: tagEventImpl{
			Base:  event.NewBase(event.EventTypeTagUpdated, opts...),
			tagID: aTag.ID(),
		},
		tag: aTag,
	}
}

func (e tagUpdatedEventImpl) Tag() Tag {
	return e.tag
}
