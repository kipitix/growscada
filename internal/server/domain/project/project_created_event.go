package project

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ProjectCreatedEvent is recorded by CreateProject.
type ProjectCreatedEvent interface {
	ProjectEvent
}

type projectCreatedEventImpl struct {
	projectEventImpl
}

var _ ProjectCreatedEvent = (*projectCreatedEventImpl)(nil)

func NewProjectCreatedEvent(anID id.ID[Project], opts ...event.EventOption) ProjectCreatedEvent {
	return &projectCreatedEventImpl{
		projectEventImpl: projectEventImpl{
			Base:      event.NewBase(event.EventTypeProjectCreated, opts...),
			projectID: anID,
		},
	}
}
