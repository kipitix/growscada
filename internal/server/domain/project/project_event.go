package project

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// ProjectEvent - base interface for the events a Project records about
// itself. A ContentEvent carries a ProjectID too but is no ProjectEvent:
// projectEvent tells them apart (and contentEvent the other way).
type ProjectEvent interface {
	event.Event
	ProjectID() id.ID[Project]
	projectEvent()
}

// ContentEvent - an event about the content of a Project: a Tag, a Scene or a
// Widget. It names the Project the content belongs to, so the event says on
// its own which Project it describes (ADR 0006). A ProjectEvent carries a
// ProjectID too but is no ContentEvent: contentEvent, which only Content
// gives, tells them apart, so a type switch need not order its cases.
type ContentEvent interface {
	event.Event
	ProjectID() id.ID[Project]
	contentEvent()
}

// Content is embedded by the events of a Project's content: it names their
// Project and makes them ContentEvents.
type Content struct {
	projectID id.ID[Project]
}

// NewContent returns the Content of an event of the Project aProjectID.
func NewContent(aProjectID id.ID[Project]) Content {
	return Content{projectID: aProjectID}
}

func (c Content) ProjectID() id.ID[Project] {
	return c.projectID
}

func (Content) contentEvent() {}

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
