package library

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/description"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Library - aggregate representing a reusable collection of WidgetTypes,
// owned by an Organization and outside every Project (ADR 0007). Its Draft is
// the Draft aggregate of the Library (ADR 0016): the WidgetTypes are its
// entities and every write of them states the Draft's one Version. Projects
// never see the Draft, only the Library Releases they pin.
//
// Its name and description are attributes of the aggregate, never content: a
// Library Release and a pin carry the WidgetType copies alone (task 73).
//
// Deleting a Library only marks it deleted, and only while no live Project
// pins it: from then on it does not exist for anyone but the policy that
// removes it with its Draft and its releases.
//
// A Discard is one transaction over the Draft (ADR 0016): it brings the
// WidgetTypes back to the latest Library Release, with no mark and no policy.
//
// Library is immutable: every change returns a new Library that also records
// the change's event. A change keeps the Version; the repository raises it
// once when it saves the result.
type Library interface {
	ID() id.ID[Library]
	OrganizationID() id.ID[organization.Organization]
	Name() LibraryName
	// Description is the library's optional, free-text description; empty
	// when it has none. It is an attribute of the aggregate, never content.
	Description() description.Description
	Version() version.Version[Library]
	// IsDeleted reports whether the library is marked deleted (Delete).
	IsDeleted() bool

	// CheckVersion reports an edit conflict: the caller changes the library
	// having seen it at a Version other than the current one.
	// Returns ErrLibraryConflict if expected is not the Version.
	CheckVersion(expected version.Version[Library]) error

	// Update changes the library's name and description and records a
	// LibraryUpdatedEvent. Returns ErrLibraryConflict if expected is not the
	// Version.
	Update(expected version.Version[Library], aName LibraryName, aDescription description.Description) (Library, error)

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
	description    description.Description
	deleted        bool
	version        version.Version[Library]
	pending        []event.Event
}

var _ Library = (*libraryImpl)(nil)

// CreateLibrary creates a new, not yet saved library of the Organization
// anOrganizationID, with an empty Draft and no release, and records a
// LibraryCreatedEvent.
func CreateLibrary(anID id.ID[Library], anOrganizationID id.ID[organization.Organization], aName LibraryName, aDescription description.Description) Library {
	return &libraryImpl{
		id:             anID,
		organizationID: anOrganizationID,
		name:           aName,
		description:    aDescription,
		version:        version.Initial[Library](),
		pending:        []event.Event{NewLibraryCreatedEvent(anID)},
	}
}

// ReconstituteLibrary rebuilds a library as stored, at the given version; it
// records no event. For repositories.
func ReconstituteLibrary(
	anID id.ID[Library],
	anOrganizationID id.ID[organization.Organization],
	aName LibraryName,
	aDescription description.Description,
	deleted bool,
	aVersion version.Version[Library],
) Library {
	return &libraryImpl{
		id:             anID,
		organizationID: anOrganizationID,
		name:           aName,
		description:    aDescription,
		deleted:        deleted,
		version:        aVersion,
	}
}

func (l *libraryImpl) ID() id.ID[Library]                               { return l.id }
func (l *libraryImpl) OrganizationID() id.ID[organization.Organization] { return l.organizationID }
func (l *libraryImpl) Name() LibraryName                                { return l.name }
func (l *libraryImpl) Description() description.Description             { return l.description }
func (l *libraryImpl) Version() version.Version[Library]                { return l.version }
func (l *libraryImpl) IsDeleted() bool                                  { return l.deleted }

func (l *libraryImpl) CheckVersion(expected version.Version[Library]) error {
	if expected != l.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrLibraryConflict, expected, l.version)
	}
	return nil
}

func (l *libraryImpl) Update(expected version.Version[Library], aName LibraryName, aDescription description.Description) (Library, error) {
	if err := l.CheckVersion(expected); err != nil {
		return nil, err
	}
	updated := *l
	updated.name = aName
	updated.description = aDescription
	updated.pending = append(slices.Clone(l.pending), NewLibraryUpdatedEvent(l.id))
	return &updated, nil
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
	return fmt.Sprintf("Library: %s, Description: %q, Organization: %s, Deleted: %t, Version: %s",
		l.name, l.description, l.organizationID, l.deleted, l.version)
}
