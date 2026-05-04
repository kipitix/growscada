package event

import (
	"time"

	"github.com/kipitix/growscada/internal/domain/tag"
)

type TagCreatedEvent interface {
	Event
	ID() tag.TagID
}

type tagCreatedEventImpl struct {
	id tag.TagID
}

var _ TagCreatedEvent = (*tagCreatedEventImpl)(nil)

func NewTagCreatedEvent(id tag.TagID) TagCreatedEvent {
	return &tagCreatedEventImpl{id: id}
}

func (e tagCreatedEventImpl) Type() EventType {
	return EventTypeTagCreated
}

func (e tagCreatedEventImpl) Timestamp() EventTimestamp {
	return EventTimestamp(time.Now())
}

func (e tagCreatedEventImpl) String() string {
	return "EventTagCreated"
}

func (e tagCreatedEventImpl) ID() tag.TagID {
	return e.id
}
