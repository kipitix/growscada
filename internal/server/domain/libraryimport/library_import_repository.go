package libraryimport

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

var (
	// ErrLibraryImportNotFound is returned when the Project imports no saved
	// state of the Library.
	ErrLibraryImportNotFound = errors.New("library import not found")
	// ErrLibraryImportExists is returned by a write of a new import when the
	// Project already imports the Library: a Project imports at most one saved
	// state of a Library.
	ErrLibraryImportExists = errors.New("library is already imported")
)

// Repository reads the Library Imports of one Project (ADR 0006): every read
// sees only the imports of that Project. An import is an entity of the
// Project's Draft (ADR 0016): its writes go through the Draft's repository,
// where the Widgets it affects are checked in the same transaction.
type Repository interface {
	// FindAll returns the Project's imports in the order they were added.
	FindAll(context.Context) ([]LibraryImport, error)

	// FindByLibrary returns the Project's import of the Library.
	// Returns ErrLibraryImportNotFound if the Project imports none of it.
	FindByLibrary(context.Context, id.ID[library.Library]) (LibraryImport, error)

	// FindImportedWidgetTypes returns the WidgetTypes of every saved state the
	// Project imports: what its Widgets may be of.
	FindImportedWidgetTypes(context.Context) ([]library.WidgetTypeCopy, error)

	// FindImportedWidgetTypesByID returns the WidgetTypes with the given IDs
	// from the saved states the Project imports; an ID none of them holds is left
	// out.
	FindImportedWidgetTypesByID(context.Context, []id.ID[library.WidgetType]) ([]library.WidgetTypeCopy, error)

	// FindImportedWidgetType returns the WidgetType with the given ID from the
	// saved states the Project imports.
	// Returns library.ErrWidgetTypeNotFound if none of them holds it.
	FindImportedWidgetType(context.Context, id.ID[library.WidgetType]) (library.WidgetTypeCopy, error)
}
