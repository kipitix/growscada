package librarypin

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// LibraryPinEvent - base interface for all events a LibraryPin records. It
// is a project.ContentEvent: it names the pin's Project, and its Library is
// what tells the pin apart in it.
type LibraryPinEvent interface {
	project.ContentEvent
	LibraryID() id.ID[library.Library]
}

type libraryPinEventImpl struct {
	event.Base
	libraryID id.ID[library.Library]
	project.Content
}

var _ LibraryPinEvent = (*libraryPinEventImpl)(nil)

// NO FABRIC METHOD

func (e libraryPinEventImpl) LibraryID() id.ID[library.Library] {
	return e.libraryID
}

func newLibraryPinEvent(anEventType event.EventType, aLibraryID id.ID[library.Library], aProjectID id.ID[project.Project], opts ...event.EventOption) libraryPinEventImpl {
	return libraryPinEventImpl{Base: event.NewBase(anEventType, opts...), libraryID: aLibraryID, Content: project.NewContent(aProjectID)}
}

// LibraryPinCreatedEvent is recorded by CreateLibraryPin.
type LibraryPinCreatedEvent interface{ LibraryPinEvent }

// LibraryPinUpdatedEvent is recorded by LibraryPin.Switch.
type LibraryPinUpdatedEvent interface{ LibraryPinEvent }

// LibraryPinDeletedEvent is recorded by LibraryPin.Delete.
type LibraryPinDeletedEvent interface{ LibraryPinEvent }

type libraryPinCreatedEventImpl struct{ libraryPinEventImpl }
type libraryPinUpdatedEventImpl struct{ libraryPinEventImpl }
type libraryPinDeletedEventImpl struct{ libraryPinEventImpl }

func NewLibraryPinCreatedEvent(aLibraryID id.ID[library.Library], aProjectID id.ID[project.Project], opts ...event.EventOption) LibraryPinCreatedEvent {
	return &libraryPinCreatedEventImpl{newLibraryPinEvent(event.EventTypeLibraryPinCreated, aLibraryID, aProjectID, opts...)}
}

func NewLibraryPinUpdatedEvent(aLibraryID id.ID[library.Library], aProjectID id.ID[project.Project], opts ...event.EventOption) LibraryPinUpdatedEvent {
	return &libraryPinUpdatedEventImpl{newLibraryPinEvent(event.EventTypeLibraryPinUpdated, aLibraryID, aProjectID, opts...)}
}

func NewLibraryPinDeletedEvent(aLibraryID id.ID[library.Library], aProjectID id.ID[project.Project], opts ...event.EventOption) LibraryPinDeletedEvent {
	return &libraryPinDeletedEventImpl{newLibraryPinEvent(event.EventTypeLibraryPinDeleted, aLibraryID, aProjectID, opts...)}
}
