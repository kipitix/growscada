package library

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Library - aggregate representing a reusable collection of WidgetTypes,
// owned by an Organization and outside every Project (ADR 0007). Its Draft
// is its WidgetTypes, each an aggregate of its own carrying the Library's ID;
// Projects never see the Draft, only the Library Releases they pin.
//
// Deleting a Library only marks it deleted, and only while no live Project
// pins it: from then on it does not exist for anyone but the policy that
// removes it with its Draft and its releases.
//
// A Discard marks the Library as being reset to its latest Library Release
// (DiscardingTo); a policy brings every WidgetType back to that release and
// clears the mark last (ADR 0012). While the mark stands the Draft is
// read-only.
//
// Library is immutable: every change returns a new Library that also records
// the change's event. A change keeps the Version; the repository raises it
// once when it saves the result.
type Library interface {
	ID() id.ID[Library]
	OrganizationID() id.ID[organization.Organization]
	Name() LibraryName
	Version() version.Version[Library]
	// IsDeleted reports whether the library is marked deleted (Delete).
	IsDeleted() bool
	// DiscardingTo returns the Library Release the Draft is being reset to
	// and true while a Discard is under way.
	DiscardingTo() (ReleaseNumber, bool)

	// CheckVersion reports an edit conflict: the caller changes the library
	// having seen it at a Version other than the current one.
	// Returns ErrLibraryConflict if expected is not the Version.
	CheckVersion(expected version.Version[Library]) error

	// CheckEditable reports whether the Draft may change: not while a
	// Discard is under way (ErrLibraryDiscarding).
	CheckEditable() error

	// Rename changes the library's name and records a LibraryUpdatedEvent.
	// Returns ErrLibraryConflict if expected is not the Version.
	Rename(expected version.Version[Library], aName LibraryName) (Library, error)

	// StartDiscard marks the library as being reset to its latest Library
	// Release, latest, and records a LibraryDiscardStartedEvent. It expects
	// no Version, as a removal: it rests on no state a client saw.
	// Returns ErrNoRelease if the library has no release yet (latest is
	// zero), ErrLibraryDiscarding if a Discard is already under way.
	StartDiscard(latest ReleaseNumber) (Library, error)

	// FinishDiscard clears the mark of StartDiscard and records a
	// LibraryUpdatedEvent; a library not being reset is returned as it is.
	FinishDiscard() Library

	// Delete marks the library deleted and records a LibraryDeletedEvent;
	// saving it frees its name. Like removing any aggregate, it expects no
	// Version.
	Delete() Library

	event.Recorder
	fmt.Stringer
}

// libraryImpl - Library implementation struct.
type libraryImpl struct {
	id             id.ID[Library]
	organizationID id.ID[organization.Organization]
	name           LibraryName
	deleted        bool
	discardingTo   ReleaseNumber
	version        version.Version[Library]
	pending        []event.Event
}

var _ Library = (*libraryImpl)(nil)

// CreateLibrary creates a new, not yet saved library of the Organization
// anOrganizationID, with an empty Draft and no release, and records a
// LibraryCreatedEvent.
func CreateLibrary(anID id.ID[Library], anOrganizationID id.ID[organization.Organization], aName LibraryName) Library {
	return &libraryImpl{
		id:             anID,
		organizationID: anOrganizationID,
		name:           aName,
		version:        version.Initial[Library](),
		pending:        []event.Event{NewLibraryCreatedEvent(anID)},
	}
}

// ReconstituteLibrary rebuilds a library as stored, at the given version; it
// records no event. For repositories. discardingTo is the zero
// ReleaseNumber unless a Discard is under way.
func ReconstituteLibrary(
	anID id.ID[Library],
	anOrganizationID id.ID[organization.Organization],
	aName LibraryName,
	deleted bool,
	discardingTo ReleaseNumber,
	aVersion version.Version[Library],
) Library {
	return &libraryImpl{
		id:             anID,
		organizationID: anOrganizationID,
		name:           aName,
		deleted:        deleted,
		discardingTo:   discardingTo,
		version:        aVersion,
	}
}

func (l *libraryImpl) ID() id.ID[Library]                               { return l.id }
func (l *libraryImpl) OrganizationID() id.ID[organization.Organization] { return l.organizationID }
func (l *libraryImpl) Name() LibraryName                                { return l.name }
func (l *libraryImpl) Version() version.Version[Library]                { return l.version }
func (l *libraryImpl) IsDeleted() bool                                  { return l.deleted }

func (l *libraryImpl) DiscardingTo() (ReleaseNumber, bool) {
	return l.discardingTo, l.discardingTo.IsValid()
}

func (l *libraryImpl) CheckVersion(expected version.Version[Library]) error {
	if expected != l.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrLibraryConflict, expected, l.version)
	}
	return nil
}

func (l *libraryImpl) CheckEditable() error {
	if l.discardingTo.IsValid() {
		return fmt.Errorf("%w: to release %s", ErrLibraryDiscarding, l.discardingTo)
	}
	return nil
}

func (l *libraryImpl) Rename(expected version.Version[Library], aName LibraryName) (Library, error) {
	if err := l.CheckVersion(expected); err != nil {
		return nil, err
	}
	renamed := *l
	renamed.name = aName
	renamed.pending = append(slices.Clone(l.pending), NewLibraryUpdatedEvent(l.id))
	return &renamed, nil
}

func (l *libraryImpl) StartDiscard(latest ReleaseNumber) (Library, error) {
	if !latest.IsValid() {
		return nil, fmt.Errorf("cannot discard library %s: %w", l.id, ErrNoRelease)
	}
	if err := l.CheckEditable(); err != nil {
		return nil, err
	}
	marked := *l
	marked.discardingTo = latest
	marked.pending = append(slices.Clone(l.pending), NewLibraryDiscardStartedEvent(l.id))
	return &marked, nil
}

func (l *libraryImpl) FinishDiscard() Library {
	if !l.discardingTo.IsValid() {
		return l
	}
	done := *l
	done.discardingTo = ReleaseNumber{}
	done.pending = append(slices.Clone(l.pending), NewLibraryUpdatedEvent(l.id))
	return &done
}

func (l *libraryImpl) Delete() Library {
	deleted := *l
	deleted.deleted = true
	deleted.pending = append(slices.Clone(l.pending), NewLibraryDeletedEvent(l.id))
	return &deleted
}

func (l *libraryImpl) PendingEvents() []event.Event {
	return slices.Clone(l.pending)
}

// String implements [fmt.Stringer].
func (l *libraryImpl) String() string {
	return fmt.Sprintf("Library: %s, Organization: %s, Deleted: %t, DiscardingTo: %s, Version: %s",
		l.name, l.organizationID, l.deleted, l.discardingTo, l.version)
}
