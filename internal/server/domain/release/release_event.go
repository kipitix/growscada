package release

import (
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// ReleaseEvent - base interface for the events a Release records:
// release_created and release_note_updated. It names the Release's scope and
// its number; a Project's Release event also implements project.ContentEvent,
// so the event stream routes it to that Project's clients (ADR 0006,
// ADR 0009). A Library's Release publishes its own names in task 72_5.
type ReleaseEvent interface {
	event.Event
	Scope() draft.Scope
	// ReleaseNumber is the number of the Release within its scope.
	ReleaseNumber() Number
	releaseEvent()
}

// ReleaseCreatedEvent is recorded by CreateRelease: a Commit was published.
type ReleaseCreatedEvent interface{ ReleaseEvent }

// ReleaseNoteUpdatedEvent is recorded by Release.WithNote.
type ReleaseNoteUpdatedEvent interface{ ReleaseEvent }

// projectReleaseEventImpl is a Project's Release event: embedding
// project.Content makes it a project.ContentEvent.
type projectReleaseEventImpl struct {
	event.Base
	project.Content
	number Number
}

var (
	_ ReleaseCreatedEvent     = (*projectReleaseEventImpl)(nil)
	_ ReleaseNoteUpdatedEvent = (*projectReleaseEventImpl)(nil)
	_ project.ContentEvent    = (*projectReleaseEventImpl)(nil)
)

func (e projectReleaseEventImpl) Scope() draft.Scope {
	return draft.ProjectScope(e.ProjectID())
}

func (e projectReleaseEventImpl) ReleaseNumber() Number { return e.number }
func (projectReleaseEventImpl) releaseEvent()           {}

// libraryReleaseEventImpl is a Library's Release event: it names the Library
// and goes to the server's stream like the Library's other events.
type libraryReleaseEventImpl struct {
	event.Base
	libraryID id.ID[library.Library]
	number    Number
}

var (
	_ ReleaseCreatedEvent     = (*libraryReleaseEventImpl)(nil)
	_ ReleaseNoteUpdatedEvent = (*libraryReleaseEventImpl)(nil)
)

func (e libraryReleaseEventImpl) Scope() draft.Scope {
	return draft.LibraryScope(e.libraryID)
}

func (e libraryReleaseEventImpl) ReleaseNumber() Number { return e.number }
func (libraryReleaseEventImpl) releaseEvent()           {}

// LibraryID is the Library of a Library's Release event; a zero ID for a
// Project's.
func (e libraryReleaseEventImpl) LibraryID() id.ID[library.Library] { return e.libraryID }

// NewReleaseCreatedEvent returns the release_created event of the Release
// number of scope.
func NewReleaseCreatedEvent(scope draft.Scope, number Number, opts ...event.EventOption) ReleaseCreatedEvent {
	return newReleaseEvent(event.EventTypeReleaseCreated, scope, number, opts...).(ReleaseCreatedEvent)
}

// NewReleaseNoteUpdatedEvent returns the release_note_updated event of the
// Release number of scope.
func NewReleaseNoteUpdatedEvent(scope draft.Scope, number Number, opts ...event.EventOption) ReleaseNoteUpdatedEvent {
	return newReleaseEvent(event.EventTypeReleaseNoteUpdated, scope, number, opts...).(ReleaseNoteUpdatedEvent)
}

// newReleaseEvent builds the Release event of the type for the scope: a
// Project's embeds its Content, a Library's names the Library.
func newReleaseEvent(eventType event.EventType, scope draft.Scope, number Number, opts ...event.EventOption) ReleaseEvent {
	if scope.IsLibrary() {
		return &libraryReleaseEventImpl{
			Base:      event.NewBase(eventType, opts...),
			libraryID: scope.LibraryID(),
			number:    number,
		}
	}
	return &projectReleaseEventImpl{
		Base:    event.NewBase(eventType, opts...),
		Content: project.NewContent(scope.ProjectID()),
		number:  number,
	}
}
