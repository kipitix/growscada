package project

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ProjectEvent - base interface for the events a Project records about
// itself. A ContentEvent carries a ProjectID too but is no ProjectEvent:
// projectEvent tells them apart.
type ProjectEvent interface {
	event.Event
	ProjectID() id.ID[Project]
	projectEvent()
}

// ContentEvent - an event about the content of a Project: a Tag, a Scene or a
// Widget. It names the Project the content belongs to, so the event says on
// its own which Project it describes (ADR 0006). Every ProjectEvent has the
// method too, so a type switch takes ProjectEvent first.
type ContentEvent interface {
	event.Event
	ProjectID() id.ID[Project]
}

type projectEventImpl struct {
	event.Base
	projectID id.ID[Project]
}

var _ ProjectEvent = (*projectEventImpl)(nil)

// NO FABRIC METHOD

func (e projectEventImpl) ProjectID() id.ID[Project] {
	return e.projectID
}

func (projectEventImpl) projectEvent() {}
