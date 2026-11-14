package libraryimport

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// LibraryImportEvent - base interface for all events a LibraryImport records.
// It is a project.ContentEvent: it names the import's Project, and its Library
// is what tells the import apart in it.
type LibraryImportEvent interface {
	project.ContentEvent
	LibraryID() id.ID[library.Library]
}

type libraryImportEventImpl struct {
	event.Base
	libraryID id.ID[library.Library]
	project.Content
}

var _ LibraryImportEvent = (*libraryImportEventImpl)(nil)

// NO FABRIC METHOD

func (e libraryImportEventImpl) LibraryID() id.ID[library.Library] {
	return e.libraryID
}

func newLibraryImportEvent(anEventType event.EventType, aLibraryID id.ID[library.Library], aProjectID id.ID[project.Project], opts ...event.EventOption) libraryImportEventImpl {
	return libraryImportEventImpl{Base: event.NewBase(anEventType, opts...), libraryID: aLibraryID, Content: project.NewContent(aProjectID)}
}

// LibraryImportCreatedEvent is recorded by CreateLibraryImport.
type LibraryImportCreatedEvent interface{ LibraryImportEvent }

// LibraryImportUpdatedEvent is recorded by LibraryImport.Switch.
type LibraryImportUpdatedEvent interface{ LibraryImportEvent }

// LibraryImportDeletedEvent is recorded by LibraryImport.Delete.
type LibraryImportDeletedEvent interface{ LibraryImportEvent }

type libraryImportCreatedEventImpl struct{ libraryImportEventImpl }
type libraryImportUpdatedEventImpl struct{ libraryImportEventImpl }
type libraryImportDeletedEventImpl struct{ libraryImportEventImpl }

func NewLibraryImportCreatedEvent(aLibraryID id.ID[library.Library], aProjectID id.ID[project.Project], opts ...event.EventOption) LibraryImportCreatedEvent {
	return &libraryImportCreatedEventImpl{newLibraryImportEvent(event.EventTypeLibraryImportCreated, aLibraryID, aProjectID, opts...)}
}

func NewLibraryImportUpdatedEvent(aLibraryID id.ID[library.Library], aProjectID id.ID[project.Project], opts ...event.EventOption) LibraryImportUpdatedEvent {
	return &libraryImportUpdatedEventImpl{newLibraryImportEvent(event.EventTypeLibraryImportUpdated, aLibraryID, aProjectID, opts...)}
}

func NewLibraryImportDeletedEvent(aLibraryID id.ID[library.Library], aProjectID id.ID[project.Project], opts ...event.EventOption) LibraryImportDeletedEvent {
	return &libraryImportDeletedEventImpl{newLibraryImportEvent(event.EventTypeLibraryImportDeleted, aLibraryID, aProjectID, opts...)}
}
