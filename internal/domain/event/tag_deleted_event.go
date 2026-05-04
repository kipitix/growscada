package event

import (
	"time"

	"github.com/kipitix/growscada/internal/domain/tag"
)

type TagDeletedEvent interface {
	Event
	TagID() tag.TagID
}

type tagDeletedEventImpl struct {
	tagID tag.TagID
}

var _ TagDeletedEvent = (*tagDeletedEventImpl)(nil)

func NewTagDeletedEvent(aTagID tag.TagID) TagDeletedEvent {
	return &tagDeletedEventImpl{tagID: aTagID}
}

func (e tagDeletedEventImpl) Type() EventType {
	return EventTypeTagDeleted
}

func (e tagDeletedEventImpl) Timestamp() EventTimestamp {
	return EventTimestamp(time.Now())
}

func (e tagDeletedEventImpl) String() string {
	return "EventTagDeleted"
}

func (e tagDeletedEventImpl) TagID() tag.TagID {
	return e.tagID
}
