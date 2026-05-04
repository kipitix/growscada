package event

import (
	"time"

	"github.com/kipitix/growscada/internal/domain/tag"
)

type TagUpdatedEvent interface {
	Event
	ID() tag.TagID
}

type tagUpdatedEventImpl struct {
	id tag.TagID
}

var _ TagUpdatedEvent = (*tagUpdatedEventImpl)(nil)

func NewTagUpdatedEvent(id tag.TagID) TagUpdatedEvent {
	return &tagUpdatedEventImpl{id: id}
}

func (e tagUpdatedEventImpl) Type() EventType {
	return EventTypeTagCreated
}

func (e tagUpdatedEventImpl) Timestamp() EventTimestamp {
	return EventTimestamp(time.Now())
}

func (e tagUpdatedEventImpl) String() string {
	return "EventTagUpdated"
}

func (e tagUpdatedEventImpl) ID() tag.TagID {
	return e.id
}
