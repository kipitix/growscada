package draft

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// SavedStateEvent - base interface for the events a saved state records:
// saved_state_created and saved_state_title_updated. It names the Draft's Scope and the
// saved state's number; a Project's saved state event also implements
// project.ContentEvent, so the event stream routes it to that Project's
// clients, while a Library's goes to the server's stream like the Library's
// other events (ADR 0006, ADR 0009).
type SavedStateEvent interface {
	event.Event
	Scope() Scope
	// SavedStateNumber is the number of the saved state within its Draft.
	SavedStateNumber() int
	savedStateEvent()
}

// SavedStateCreatedEvent is recorded by NewSavedState: a Save fixed the Draft.
type SavedStateCreatedEvent interface{ SavedStateEvent }

// SavedStateTitleUpdatedEvent is recorded by SavedState.WithTitle.
type SavedStateTitleUpdatedEvent interface{ SavedStateEvent }

// projectSavedStateEventImpl is a Project's saved state event: embedding
// project.Content makes it a project.ContentEvent, and scopeOf reads the
// Scope back from it.
type projectSavedStateEventImpl struct {
	event.Base
	project.Content
	number int
}

var (
	_ SavedStateCreatedEvent      = (*projectSavedStateEventImpl)(nil)
	_ SavedStateTitleUpdatedEvent = (*projectSavedStateEventImpl)(nil)
	_ project.ContentEvent        = (*projectSavedStateEventImpl)(nil)
)

func (e projectSavedStateEventImpl) Scope() Scope {
	return ProjectScope(e.ProjectID())
}

func (e projectSavedStateEventImpl) SavedStateNumber() int { return e.number }
func (projectSavedStateEventImpl) savedStateEvent()        {}

// librarySavedStateEventImpl is a Library's saved state event: it names the
// Library but is no library.LibraryEvent (that interface's marker is private
// to the library package); it goes to the server's stream by default.
type librarySavedStateEventImpl struct {
	event.Base
	libraryID id.ID[library.Library]
	number    int
}

var (
	_ SavedStateCreatedEvent      = (*librarySavedStateEventImpl)(nil)
	_ SavedStateTitleUpdatedEvent = (*librarySavedStateEventImpl)(nil)
)

func (e librarySavedStateEventImpl) Scope() Scope {
	return LibraryScope(e.libraryID)
}

func (e librarySavedStateEventImpl) SavedStateNumber() int { return e.number }
func (librarySavedStateEventImpl) savedStateEvent()        {}

// LibraryID is the Library of a Library's saved state event; a zero ID for a
// Project's.
func (e librarySavedStateEventImpl) LibraryID() id.ID[library.Library] { return e.libraryID }

// NewSavedStateCreatedEvent returns the saved_state_created event of the saved state
// number of scope.
func NewSavedStateCreatedEvent(scope Scope, number int, opts ...event.EventOption) SavedStateCreatedEvent {
	return newSavedStateEvent(event.EventTypeSavedStateCreated, scope, number, opts...).(SavedStateCreatedEvent)
}

// NewSavedStateTitleUpdatedEvent returns the saved_state_title_updated event of the
// saved state number of scope.
func NewSavedStateTitleUpdatedEvent(scope Scope, number int, opts ...event.EventOption) SavedStateTitleUpdatedEvent {
	return newSavedStateEvent(event.EventTypeSavedStateTitleUpdated, scope, number, opts...).(SavedStateTitleUpdatedEvent)
}

// newSavedStateEvent builds the saved state event of the type for the scope: a
// Project's embeds its Content, a Library's names the Library.
func newSavedStateEvent(eventType event.EventType, scope Scope, number int, opts ...event.EventOption) SavedStateEvent {
	if scope.IsLibrary() {
		return &librarySavedStateEventImpl{
			Base:      event.NewBase(eventType, opts...),
			libraryID: scope.LibraryID(),
			number:    number,
		}
	}
	return &projectSavedStateEventImpl{
		Base:    event.NewBase(eventType, opts...),
		Content: project.NewContent(scope.ProjectID()),
		number:  number,
	}
}
