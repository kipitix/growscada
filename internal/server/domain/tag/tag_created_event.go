package tag

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// TagCreatedEvent is recorded by CreateTag.
type TagCreatedEvent interface {
	TagEvent
}

type tagCreatedEventImpl struct {
	tagEventImpl
}

var _ TagCreatedEvent = (*tagCreatedEventImpl)(nil)

func NewTagCreatedEvent(anID id.ID[Tag], aProjectID id.ID[project.Project], opts ...event.EventOption) TagCreatedEvent {
	return &tagCreatedEventImpl{
		tagEventImpl: tagEventImpl{
			Base:    event.NewBase(event.EventTypeTagCreated, opts...),
			tagID:   anID,
			Content: project.NewContent(aProjectID),
		},
	}
}
