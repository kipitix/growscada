package project

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ProjectUpdatedEvent is recorded by Project.Rename.
type ProjectUpdatedEvent interface {
	ProjectEvent
}

type projectUpdatedEventImpl struct {
	projectEventImpl
}

var _ ProjectUpdatedEvent = (*projectUpdatedEventImpl)(nil)

func NewProjectUpdatedEvent(anID id.ID[Project], opts ...event.EventOption) ProjectUpdatedEvent {
	return &projectUpdatedEventImpl{
		projectEventImpl: projectEventImpl{
			Base:      event.NewBase(event.EventTypeProjectUpdated, opts...),
			projectID: anID,
		},
	}
}
