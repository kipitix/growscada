package project

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ProjectDiscardStartedEvent is recorded by Project.StartDiscard: the Draft
// is being reset to the Revision the Project runs (task 35, Q10).
type ProjectDiscardStartedEvent interface {
	ProjectEvent
}

type projectDiscardStartedEventImpl struct {
	projectEventImpl
}

var _ ProjectDiscardStartedEvent = (*projectDiscardStartedEventImpl)(nil)

func NewProjectDiscardStartedEvent(anID id.ID[Project], opts ...event.EventOption) ProjectDiscardStartedEvent {
	return &projectDiscardStartedEventImpl{
		projectEventImpl: projectEventImpl{
			Base:      event.NewBase(event.EventTypeProjectDiscardStarted, opts...),
			projectID: anID,
		},
	}
}
