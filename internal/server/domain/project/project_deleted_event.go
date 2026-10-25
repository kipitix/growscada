package project

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ProjectDeletedEvent is recorded by Project.Delete.
type ProjectDeletedEvent interface {
	ProjectEvent
}

type projectDeletedEventImpl struct {
	projectEventImpl
}

var _ ProjectDeletedEvent = (*projectDeletedEventImpl)(nil)

func NewProjectDeletedEvent(anID id.ID[Project], opts ...event.EventOption) ProjectDeletedEvent {
	return &projectDeletedEventImpl{
		projectEventImpl: projectEventImpl{
			Base:      event.NewBase(event.EventTypeProjectDeleted, opts...),
			projectID: anID,
		},
	}
}
