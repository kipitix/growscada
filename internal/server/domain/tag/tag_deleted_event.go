package tag

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// TagDeletedEvent is recorded by Tag.Delete.
type TagDeletedEvent interface {
	TagEvent
}

type tagDeletedEventImpl struct {
	tagEventImpl
}

var _ TagDeletedEvent = (*tagDeletedEventImpl)(nil)

func NewTagDeletedEvent(anID id.ID[Tag], opts ...event.EventOption) TagDeletedEvent {
	return &tagDeletedEventImpl{
		tagEventImpl: tagEventImpl{
			Base:  event.NewBase(event.EventTypeTagDeleted, opts...),
			tagID: anID,
		},
	}
}
