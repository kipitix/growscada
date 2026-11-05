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
// records the change's event. It is an entity of its Project's Draft, which
// carries the one Version of the Draft's content (ADR 0016).
type LibraryPin interface {
	// ProjectID is the Project whose Draft holds the pin; it never changes.
	ProjectID() id.ID[project.Project]
	// LibraryID is the Library pinned; it never changes.
	LibraryID() id.ID[library.Library]
	// Release is the number of the Library Release pinned.
	Release() library.ReleaseNumber

	// Switch pins the release instead and records a LibraryPinUpdatedEvent.
	// Returns an error if release is of another Library.
	Switch(release library.LibraryRelease) (LibraryPin, error)

	// Delete records a LibraryPinDeletedEvent; the Draft's write removes the
	// pin.
	Delete() LibraryPin

	event.Recorder
	fmt.Stringer
}

type libraryPinImpl struct {
	projectID id.ID[project.Project]
	libraryID id.ID[library.Library]
	release   library.ReleaseNumber
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
		pending:   []event.Event{NewLibraryPinCreatedEvent(release.LibraryID(), aProjectID)},
	}
}

// ReconstituteLibraryPin rebuilds a pin as stored; it records no event. For
// repositories.
func ReconstituteLibraryPin(aProjectID id.ID[project.Project], aLibraryID id.ID[library.Library], aRelease library.ReleaseNumber) LibraryPin {
	return &libraryPinImpl{projectID: aProjectID, libraryID: aLibraryID, release: aRelease}
}

func (p *libraryPinImpl) ProjectID() id.ID[project.Project] { return p.projectID }
func (p *libraryPinImpl) LibraryID() id.ID[library.Library] { return p.libraryID }
func (p *libraryPinImpl) Release() library.ReleaseNumber    { return p.release }
func (p *libraryPinImpl) Switch(release library.LibraryRelease) (LibraryPin, error) {
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
	return fmt.Sprintf("LibraryPin: library %s release %s in project %s", p.libraryID, p.release, p.projectID)
}
