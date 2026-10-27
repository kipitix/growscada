package librarypin

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

var (
	// ErrLibraryPinNotFound is returned when the Project does not pin the
	// Library.
	ErrLibraryPinNotFound = errors.New("library pin not found")
	// ErrLibraryPinConflict is returned by the LibraryPin's change methods
	// when the expected version is not the current one (an edit conflict),
	// and by Save and Delete when the stored version changed after the pin
	// was read (a write race).
	ErrLibraryPinConflict = errors.New("library pin version conflict")
	// ErrLibraryPinExists is returned by Save of a new pin when the Project
	// already pins the Library: a Project pins at most one release of a
	// Library.
	ErrLibraryPinExists = errors.New("library already pinned")
)

// WidgetsCheck checks a switch or removal of a pin against the Project's
// Widgets, read in its transaction once the pin's row is locked: no Widget
// added or retyped concurrently slips past it (ADR 0012).
type WidgetsCheck func(widgets []PlacedWidget) error

// Repository - repository interface for storing and managing the Library
// Pins of one Project (ADR 0006): every read sees only the pins of that
// Project, and saving or deleting one of another Project is a programming
// error.
type Repository interface {
	// FindAll returns the Project's pins in the order they were added.
	FindAll(context.Context) ([]LibraryPin, error)

	// FindByLibrary returns the Project's pin of the Library.
	// Returns ErrLibraryPinNotFound if the Project does not pin it.
	FindByLibrary(context.Context, id.ID[library.Library]) (LibraryPin, error)

	// Save stores a pin and the events it recorded, in one transaction. A
	// pin of the initial version is inserted while the Project and the
	// Library are live (each row locked for share, so neither is deleted
	// meanwhile); any other replaces the stored one if the stored version is
	// still the pin's version, once check passed against the Project's
	// Widgets with the pin's row locked for update. The version is raised
	// once per Save; the returned pin carries the new one and no events.
	// Returns ErrLibraryPinExists if the Project already pins the Library on
	// insert, ErrLibraryPinNotFound if the pin does not exist on update,
	// ErrLibraryPinConflict if the stored version does not match,
	// library.ErrLibraryNotFound if the Library is not live,
	// library.ErrReleaseNotFound if it has no such release,
	// project.ErrProjectNotFound if the Project is not live, or check's
	// error.
	Save(ctx context.Context, pin LibraryPin, check WidgetsCheck) (LibraryPin, error)

	// Delete removes a pin if the stored version is still the pin's version,
	// and stores the events it recorded (see LibraryPin.Delete), once check
	// passed against the Project's Widgets with the pin's row locked for
	// update: a pin switched since it was read is not checked against the
	// release it pinned before.
	// Returns ErrLibraryPinNotFound if the pin does not exist,
	// ErrLibraryPinConflict if the stored version does not match, or check's
	// error.
	Delete(ctx context.Context, pin LibraryPin, check WidgetsCheck) error

	// FindPinnedWidgetTypes returns the WidgetTypes of every Library
	// Release the Project pins: what its Widgets may be of.
	FindPinnedWidgetTypes(context.Context) ([]library.ReleasedWidgetType, error)

	// FindPinnedWidgetTypesByID returns the WidgetTypes with the given IDs
	// from the Library Releases the Project pins; an ID none of them holds
	// is left out.
	FindPinnedWidgetTypesByID(context.Context, []id.ID[library.WidgetType]) ([]library.ReleasedWidgetType, error)

	// FindPinnedWidgetType returns the WidgetType with the given ID from the
	// Library Releases the Project pins.
	// Returns library.ErrWidgetTypeNotFound if none of them holds it.
	FindPinnedWidgetType(context.Context, id.ID[library.WidgetType]) (library.ReleasedWidgetType, error)
}
