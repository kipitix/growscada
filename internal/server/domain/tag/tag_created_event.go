package tag

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// TagCreatedEvent is recorded by CreateTag.
type TagCreatedEvent interface {
	TagEvent
}

type tagCreatedEventImpl struct {
	tagEventImpl
}

var _ TagCreatedEvent = (*tagCreatedEventImpl)(nil)

func NewTagCreatedEvent(anID id.ID[Tag], opts ...event.EventOption) TagCreatedEvent {
	return &tagCreatedEventImpl{
		tagEventImpl: tagEventImpl{
			Base:  event.NewBase(event.EventTypeTagCreated, opts...),
			tagID: anID,
		},
	}
}
