// Package librarypin holds the Library Pin: the choice, in a Project's
// Draft, of one Library Release of one Library (ADR 0007, ADR 0012).
package librarypin

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// LibraryPin - aggregate of a Project's Draft, one per pair of the Project
// and a Library: the Library Release of that Library whose WidgetTypes the
// Project uses. It is identified by its Library within its Project.
//
// Switching or removing a pin is refused while Widgets of the Project would
// be left without their WidgetType (Impact); the PortBindings it breaks are
// removed by a policy afterwards (ADR 0012).
//
// LibraryPin is immutable: every change returns a new LibraryPin that also
// records the change's event. A change keeps the Version; the repository
// raises it once when it saves the result.
type LibraryPin interface {
	// ProjectID is the Project whose Draft holds the pin; it never changes.
	ProjectID() id.ID[project.Project]
	// LibraryID is the Library pinned; it never changes.
	LibraryID() id.ID[library.Library]
	// Release is the number of the Library Release pinned.
	Release() library.ReleaseNumber
	Version() version.Version[LibraryPin]

	// CheckVersion reports an edit conflict: the caller changes the pin
	// having seen it at a Version other than the current one.
	// Returns ErrLibraryPinConflict if expected is not the Version.
	CheckVersion(expected version.Version[LibraryPin]) error

	// Switch pins the release instead and records a LibraryPinUpdatedEvent.
	// Returns ErrLibraryPinConflict if expected is not the Version, and an
	// error if release is of another Library.
	Switch(expected version.Version[LibraryPin], release library.LibraryRelease) (LibraryPin, error)

	// Delete records a LibraryPinDeletedEvent; the repository's Delete
	// removes the pin. Like removing any aggregate, it expects no Version.
	Delete() LibraryPin

	event.Recorder
	fmt.Stringer
}

type libraryPinImpl struct {
	projectID id.ID[project.Project]
	libraryID id.ID[library.Library]
	release   library.ReleaseNumber
	version   version.Version[LibraryPin]
	pending   []event.Event
}

var _ LibraryPin = (*libraryPinImpl)(nil)

// CreateLibraryPin creates a new, not yet saved pin of the Library Release
// release in the Draft of the Project aProjectID and records a
// LibraryPinCreatedEvent.
func CreateLibraryPin(aProjectID id.ID[project.Project], release library.LibraryRelease) LibraryPin {
	return &libraryPinImpl{
		projectID: aProjectID,
		libraryID: release.LibraryID(),
		release:   release.Number(),
		version:   version.Initial[LibraryPin](),
		pending:   []event.Event{NewLibraryPinCreatedEvent(release.LibraryID(), aProjectID)},
	}
}

// ReconstituteLibraryPin rebuilds a pin as stored, at the given version; it
// records no event. For repositories.
func ReconstituteLibraryPin(aProjectID id.ID[project.Project], aLibraryID id.ID[library.Library], aRelease library.ReleaseNumber, aVersion version.Version[LibraryPin]) LibraryPin {
	return &libraryPinImpl{projectID: aProjectID, libraryID: aLibraryID, release: aRelease, version: aVersion}
}

func (p *libraryPinImpl) ProjectID() id.ID[project.Project]    { return p.projectID }
func (p *libraryPinImpl) LibraryID() id.ID[library.Library]    { return p.libraryID }
func (p *libraryPinImpl) Release() library.ReleaseNumber       { return p.release }
func (p *libraryPinImpl) Version() version.Version[LibraryPin] { return p.version }

func (p *libraryPinImpl) CheckVersion(expected version.Version[LibraryPin]) error {
	if expected != p.version {
		return fmt.Errorf("%w: expected version %s, current %s", ErrLibraryPinConflict, expected, p.version)
	}
	return nil
}

func (p *libraryPinImpl) Switch(expected version.Version[LibraryPin], release library.LibraryRelease) (LibraryPin, error) {
	if err := p.CheckVersion(expected); err != nil {
		return nil, err
	}
	if release.LibraryID() != p.libraryID {
		return nil, fmt.Errorf("cannot pin release %s of library %s in place of library %s", release.Number(), release.LibraryID(), p.libraryID)
	}
	switched := *p
	switched.release = release.Number()
	switched.pending = append(slices.Clone(p.pending), NewLibraryPinUpdatedEvent(p.libraryID, p.projectID))
	return &switched, nil
}

func (p *libraryPinImpl) Delete() LibraryPin {
	deleted := *p
	deleted.pending = append(slices.Clone(p.pending), NewLibraryPinDeletedEvent(p.libraryID, p.projectID))
	return &deleted
}

func (p *libraryPinImpl) PendingEvents() []event.Event {
	return slices.Clone(p.pending)
}

// String implements [fmt.Stringer].
func (p *libraryPinImpl) String() string {
	return fmt.Sprintf("LibraryPin: library %s release %s in project %s, Version: %s", p.libraryID, p.release, p.projectID, p.version)
}
