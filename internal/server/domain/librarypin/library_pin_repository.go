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
	// ErrLibraryPinExists is returned by Save of a new pin when the Project
	// already pins the Library: a Project pins at most one release of a
	// Library.
	ErrLibraryPinExists = errors.New("library already pinned")
)

// Repository reads the Library Pins of one Project (ADR 0006): every read
// sees only the pins of that Project. A pin is an entity of the Project's
// Draft (ADR 0016): its writes go through the Draft's repository, where the
// Widgets it affects are checked in the same transaction.
type Repository interface {
	// FindAll returns the Project's pins in the order they were added.
	FindAll(context.Context) ([]LibraryPin, error)

	// FindByLibrary returns the Project's pin of the Library.
	// Returns ErrLibraryPinNotFound if the Project does not pin it.
	FindByLibrary(context.Context, id.ID[library.Library]) (LibraryPin, error)

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
