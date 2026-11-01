package device

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// EnvironmentEvent - base interface for all events an Environment records. It
// is a project.ContentEvent: it names the Environment's Project, and the
// Environment is what tells it apart there.
type EnvironmentEvent interface {
	project.ContentEvent
	EnvironmentID() id.ID[Environment]
	environmentEvent()
}

type environmentEventImpl struct {
	event.Base
	environmentID id.ID[Environment]
	project.Content
}

var _ EnvironmentEvent = (*environmentEventImpl)(nil)

// NO FABRIC METHOD

func (e environmentEventImpl) EnvironmentID() id.ID[Environment] { return e.environmentID }

func (environmentEventImpl) environmentEvent() {}

func newEnvironmentEvent(anEventType event.EventType, anEnvironmentID id.ID[Environment], aProjectID id.ID[project.Project], opts ...event.EventOption) environmentEventImpl {
	return environmentEventImpl{Base: event.NewBase(anEventType, opts...), environmentID: anEnvironmentID, Content: project.NewContent(aProjectID)}
}

// EnvironmentCreatedEvent is recorded by CreateEnvironment.
type EnvironmentCreatedEvent interface{ EnvironmentEvent }

// EnvironmentUpdatedEvent is recorded by Environment.Rename.
type EnvironmentUpdatedEvent interface{ EnvironmentEvent }

// EnvironmentDeletedEvent is recorded by Environment.Delete.
type EnvironmentDeletedEvent interface{ EnvironmentEvent }

type environmentCreatedEventImpl struct{ environmentEventImpl }
type environmentUpdatedEventImpl struct{ environmentEventImpl }
type environmentDeletedEventImpl struct{ environmentEventImpl }

func NewEnvironmentCreatedEvent(anEnvironmentID id.ID[Environment], aProjectID id.ID[project.Project], opts ...event.EventOption) EnvironmentCreatedEvent {
	return &environmentCreatedEventImpl{newEnvironmentEvent(event.EventTypeEnvironmentCreated, anEnvironmentID, aProjectID, opts...)}
}

func NewEnvironmentUpdatedEvent(anEnvironmentID id.ID[Environment], aProjectID id.ID[project.Project], opts ...event.EventOption) EnvironmentUpdatedEvent {
	return &environmentUpdatedEventImpl{newEnvironmentEvent(event.EventTypeEnvironmentUpdated, anEnvironmentID, aProjectID, opts...)}
}

func NewEnvironmentDeletedEvent(anEnvironmentID id.ID[Environment], aProjectID id.ID[project.Project], opts ...event.EventOption) EnvironmentDeletedEvent {
	return &environmentDeletedEventImpl{newEnvironmentEvent(event.EventTypeEnvironmentDeleted, anEnvironmentID, aProjectID, opts...)}
}
