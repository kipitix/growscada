// Package libraryimport holds the Library Import: the choice, in a Project's
// Draft, of one saved state of one Library (task 85_6; ADR 0007, ADR 0012). A
// Project sees the WidgetTypes of the saved state it imported: the copies the
// saved state holds, never the Library's Draft.
package libraryimport

import (
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// LibraryImport - aggregate of a Project's Draft, one per pair of the Project
// and a Library: the Library's saved state whose WidgetTypes the Project uses.
// It is identified by its Library within its Project.
//
// Switching or removing an import is refused while Widgets of the Project would
// be left without their WidgetType (Impact); the PortBindings it breaks are
// removed in the same change (ADR 0012).
//
// LibraryImport is immutable: every change returns a new LibraryImport that
// also records the change's event. It is an entity of its Project's Draft,
// which carries the one Version of the Draft's content (ADR 0016).
type LibraryImport interface {
	// ProjectID is the Project whose Draft holds the import; it never
	// changes.
	ProjectID() id.ID[project.Project]
	// LibraryID is the Library imported; it never changes.
	LibraryID() id.ID[library.Library]
	// SavedStateID is the identity of the imported saved state of the Library. It
	// is the identity the row carries; a value rebuilt from a ProjectFile alone
	// has none, and the write resolves it by the number.
	SavedStateID() uuid.UUID
	// SavedStateNumber is the number of the saved state imported within the
	// Library's Draft.
	SavedStateNumber() int

	// Switch imports the saved state instead and records a
	// LibraryImportUpdatedEvent. Returns an error if the saved state is of another
	// Library.
	Switch(savedState SavedState) (LibraryImport, error)

	// Delete records a LibraryImportDeletedEvent; the Draft's write removes
	// the import.
	Delete() LibraryImport

	event.Recorder
	fmt.Stringer
}

type libraryImportImpl struct {
	projectID    id.ID[project.Project]
	libraryID    id.ID[library.Library]
	savedStateID uuid.UUID
	savedState   int
	pending      []event.Event
}

var _ LibraryImport = (*libraryImportImpl)(nil)

// CreateLibraryImport creates a new, not yet saved import of a saved state in
// the Draft of the Project aProjectID and records a LibraryImportCreatedEvent.
func CreateLibraryImport(aProjectID id.ID[project.Project], savedState SavedState) LibraryImport {
	return &libraryImportImpl{
		projectID:    aProjectID,
		libraryID:    savedState.LibraryID(),
		savedStateID: savedState.ID(),
		savedState:   savedState.Number(),
		pending:      []event.Event{NewLibraryImportCreatedEvent(savedState.LibraryID(), aProjectID)},
	}
}

// ReconstituteLibraryImport rebuilds an import as stored; it records no event.
// For repositories. A saved state UUID of uuid.Nil means the identity is not
// known to the caller (an import rebuilt from a ProjectFile): the write
// resolves it by the Library and the number in the same transaction.
func ReconstituteLibraryImport(aProjectID id.ID[project.Project], aLibraryID id.ID[library.Library], aSavedStateID uuid.UUID, aSavedState int) LibraryImport {
	return &libraryImportImpl{projectID: aProjectID, libraryID: aLibraryID, savedStateID: aSavedStateID, savedState: aSavedState}
}

func (i *libraryImportImpl) ProjectID() id.ID[project.Project] { return i.projectID }
func (i *libraryImportImpl) LibraryID() id.ID[library.Library] { return i.libraryID }
func (i *libraryImportImpl) SavedStateID() uuid.UUID           { return i.savedStateID }
func (i *libraryImportImpl) SavedStateNumber() int             { return i.savedState }
func (i *libraryImportImpl) Switch(savedState SavedState) (LibraryImport, error) {
	if savedState.LibraryID() != i.libraryID {
		return nil, fmt.Errorf("cannot import saved state %d of library %s in place of library %s",
			savedState.Number(), savedState.LibraryID(), i.libraryID)
	}
	switched := *i
	switched.savedStateID = savedState.ID()
	switched.savedState = savedState.Number()
	switched.pending = append(slices.Clone(i.pending), NewLibraryImportUpdatedEvent(i.libraryID, i.projectID))
	return &switched, nil
}

func (i *libraryImportImpl) Delete() LibraryImport {
	deleted := *i
	deleted.pending = append(slices.Clone(i.pending), NewLibraryImportDeletedEvent(i.libraryID, i.projectID))
	return &deleted
}

func (i *libraryImportImpl) PendingEvents() []event.Event {
	return slices.Clone(i.pending)
}

// String implements [fmt.Stringer].
func (i *libraryImportImpl) String() string {
	return fmt.Sprintf("LibraryImport: library %s saved state %d in project %s", i.libraryID, i.savedState, i.projectID)
}
