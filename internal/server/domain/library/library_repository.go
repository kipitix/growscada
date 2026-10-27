package library

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

var (
	// ErrLibraryNotFound is returned when there is no live library with the
	// given ID or name: a library marked deleted is not found either.
	ErrLibraryNotFound = errors.New("library not found")
	// ErrLibraryConflict is returned by the Library's change methods when the
	// expected version is not the current one (an edit conflict), and by
	// Save when the stored version changed after the library was read (a
	// write race).
	ErrLibraryConflict = errors.New("library version conflict")
	// ErrLibraryNameTaken is returned by Save when another live library of
	// the same Organization already has the name.
	ErrLibraryNameTaken = errors.New("library name already taken")
	// ErrLibraryPinned is returned by Save of a library marked deleted while
	// a live Project pins one of its releases, and by Delete while any
	// Project does.
	ErrLibraryPinned = errors.New("library is pinned by projects")
	// ErrLibraryDiscarding is returned while a Discard of the library is
	// under way: its Draft is read-only (ADR 0012).
	ErrLibraryDiscarding = errors.New("library is being discarded")
	// ErrNoRelease is returned by Library.StartDiscard of a library that has
	// never been released: there is nothing to go back to.
	ErrNoRelease = errors.New("library has no release")
)

// LibraryRepository - repository interface for storing and managing
// libraries. Every read but FindDeleted sees only live libraries: one marked
// deleted does not exist for them.
type LibraryRepository interface {
	// NextID returns a new unique identifier for a library.
	NextID() id.ID[Library]

	// Save stores a library and the events it recorded, in one transaction:
	// a library of the initial version is inserted, any other replaces the
	// stored live one if the stored version is still the library's version;
	// a library marked deleted (Library.Delete) is stored so, unless a live
	// Project pins it. The version is raised once per Save; the returned
	// library carries the new one and no events.
	// Returns ErrLibraryNotFound if there is no such live library on update,
	// ErrLibraryConflict if the stored version does not match,
	// ErrLibraryNameTaken if another live library of its Organization has the
	// name, ErrLibraryPinned if it is marked deleted while pinned.
	Save(context.Context, Library) (Library, error)

	// FindByID returns the live library with the given ID.
	// Returns ErrLibraryNotFound if there is none.
	FindByID(context.Context, id.ID[Library]) (Library, error)

	// FindByName returns the live library of the Organization with the given
	// name. Returns ErrLibraryNotFound if there is none.
	FindByName(context.Context, id.ID[organization.Organization], LibraryName) (Library, error)

	// FindAll returns the live libraries of the Organization in creation
	// order.
	FindAll(context.Context, id.ID[organization.Organization]) ([]Library, error)

	// FindDeleted returns the library with the given ID if it is marked
	// deleted. Returns ErrLibraryNotFound if there is no such library, or it
	// is live.
	FindDeleted(context.Context, id.ID[Library]) (Library, error)

	// Delete removes a library marked deleted with its Draft and its Library
	// Releases. It stores no events: the library recorded its
	// LibraryDeletedEvent when it was marked.
	// Returns ErrLibraryNotFound if there is no such library marked deleted,
	// ErrLibraryPinned while Projects marked deleted, not removed yet, still
	// pin it.
	Delete(context.Context, Library) error
}
