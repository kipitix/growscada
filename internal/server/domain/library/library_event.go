package library

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// LibraryEvent - base interface for the events a Library records about
// itself: events of the server's stream, like those of Projects (no Project's
// content). The events of a WidgetType and
// of a Library Import name a Library too but are no LibraryEvent: libraryEvent
// tells them apart.
type LibraryEvent interface {
	event.Event
	LibraryID() id.ID[Library]
	libraryEvent()
}

type libraryEventImpl struct {
	event.Base
	libraryID id.ID[Library]
}

var _ LibraryEvent = (*libraryEventImpl)(nil)

// NO FABRIC METHOD

func (e libraryEventImpl) LibraryID() id.ID[Library] {
	return e.libraryID
}

func (libraryEventImpl) libraryEvent() {}

func newLibraryEvent(anEventType event.EventType, anID id.ID[Library], opts ...event.EventOption) libraryEventImpl {
	return libraryEventImpl{Base: event.NewBase(anEventType, opts...), libraryID: anID}
}

// LibraryCreatedEvent is recorded by CreateLibrary.
type LibraryCreatedEvent interface{ LibraryEvent }

// LibraryUpdatedEvent is recorded by Library.Update.
type LibraryUpdatedEvent interface{ LibraryEvent }

// LibraryDeletedEvent is recorded by Library.Delete.
type LibraryDeletedEvent interface{ LibraryEvent }

type libraryCreatedEventImpl struct{ libraryEventImpl }
type libraryUpdatedEventImpl struct{ libraryEventImpl }
type libraryDeletedEventImpl struct{ libraryEventImpl }

func NewLibraryCreatedEvent(anID id.ID[Library], opts ...event.EventOption) LibraryCreatedEvent {
	return &libraryCreatedEventImpl{newLibraryEvent(event.EventTypeLibraryCreated, anID, opts...)}
}

func NewLibraryUpdatedEvent(anID id.ID[Library], opts ...event.EventOption) LibraryUpdatedEvent {
	return &libraryUpdatedEventImpl{newLibraryEvent(event.EventTypeLibraryUpdated, anID, opts...)}
}

func NewLibraryDeletedEvent(anID id.ID[Library], opts ...event.EventOption) LibraryDeletedEvent {
	return &libraryDeletedEventImpl{newLibraryEvent(event.EventTypeLibraryDeleted, anID, opts...)}
}
