package draft

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// CommitEvent - base interface for the events a saved state records: commit_created
// and commit_title_updated. It names the Draft's Scope and the saved state's
// number; a Project's saved state event also implements project.ContentEvent, so
// the event stream routes it to that Project's clients, while a Library's
// goes to the server's stream like the Library's other events (ADR 0006,
// ADR 0009).
type CommitEvent interface {
	event.Event
	Scope() Scope
	// CommitNumber is the number of the saved state within its Draft.
	CommitNumber() int
	commitEvent()
}

// CommitCreatedEvent is recorded by NewCommit: a Save fixed the Draft.
type CommitCreatedEvent interface{ CommitEvent }

// CommitTitleUpdatedEvent is recorded by Commit.WithTitle.
type CommitTitleUpdatedEvent interface{ CommitEvent }

// projectCommitEventImpl is a Project's saved state event: embedding
// project.Content makes it a project.ContentEvent, and scopeOf reads the
// Scope back from it.
type projectCommitEventImpl struct {
	event.Base
	project.Content
	number int
}

var (
	_ CommitCreatedEvent      = (*projectCommitEventImpl)(nil)
	_ CommitTitleUpdatedEvent = (*projectCommitEventImpl)(nil)
	_ project.ContentEvent    = (*projectCommitEventImpl)(nil)
)

func (e projectCommitEventImpl) Scope() Scope {
	return ProjectScope(e.ProjectID())
}

func (e projectCommitEventImpl) CommitNumber() int { return e.number }
func (projectCommitEventImpl) commitEvent()        {}

// libraryCommitEventImpl is a Library's saved state event: it names the Library
// but is no library.LibraryEvent (that interface's marker is private to the
// library package); it goes to the server's stream by default.
type libraryCommitEventImpl struct {
	event.Base
	libraryID id.ID[library.Library]
	number    int
}

var (
	_ CommitCreatedEvent      = (*libraryCommitEventImpl)(nil)
	_ CommitTitleUpdatedEvent = (*libraryCommitEventImpl)(nil)
)

func (e libraryCommitEventImpl) Scope() Scope {
	return LibraryScope(e.libraryID)
}

func (e libraryCommitEventImpl) CommitNumber() int { return e.number }
func (libraryCommitEventImpl) commitEvent()        {}

// LibraryID is the Library of a Library's saved state event; a zero ID for a
// Project's.
func (e libraryCommitEventImpl) LibraryID() id.ID[library.Library] { return e.libraryID }

// NewCommitCreatedEvent returns the commit_created event of the saved state
// number of scope.
func NewCommitCreatedEvent(scope Scope, number int, opts ...event.EventOption) CommitCreatedEvent {
	return newCommitEvent(event.EventTypeCommitCreated, scope, number, opts...).(CommitCreatedEvent)
}

// NewCommitTitleUpdatedEvent returns the commit_title_updated event of the
// saved state number of scope.
func NewCommitTitleUpdatedEvent(scope Scope, number int, opts ...event.EventOption) CommitTitleUpdatedEvent {
	return newCommitEvent(event.EventTypeCommitTitleUpdated, scope, number, opts...).(CommitTitleUpdatedEvent)
}

// newCommitEvent builds the saved state event of the type for the scope: a
// Project's embeds its Content, a Library's names the Library.
func newCommitEvent(eventType event.EventType, scope Scope, number int, opts ...event.EventOption) CommitEvent {
	if scope.IsLibrary() {
		return &libraryCommitEventImpl{
			Base:      event.NewBase(eventType, opts...),
			libraryID: scope.LibraryID(),
			number:    number,
		}
	}
	return &projectCommitEventImpl{
		Base:    event.NewBase(eventType, opts...),
		Content: project.NewContent(scope.ProjectID()),
		number:  number,
	}
}
