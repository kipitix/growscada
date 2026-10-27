package library

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

var (
	ErrWidgetTypeNotFound = errors.New("widget type not found")
	// ErrWidgetTypeConflict is returned by the WidgetType's change methods
	// when the expected version is not the current one (an edit conflict),
	// and by Save and Delete when the stored version changed after the widget
	// type was read (a write race).
	ErrWidgetTypeConflict = errors.New("widget type version conflict")
	// ErrWidgetTypeNameTaken is returned by Save when another widget type of
	// the same Library already has the name: within its Library a widget
	// type's name is its natural key.
	ErrWidgetTypeNameTaken = errors.New("widget type name already taken")
	// ErrInvalidWidgetType is returned by WidgetType.Update when the new
	// definition breaks a rule of the widget type as a whole (e.g. two
	// InputPorts share a name).
	ErrInvalidWidgetType = errors.New("invalid widget type")
)

// WidgetTypeRepository - repository interface for storing and managing the
// WidgetTypes of the Draft of one Library: every read sees only the widget
// types of that Library, so one of another is not found, and saving or
// deleting one of another Library is a programming error.
//
// A write locks the Library's row for share and checks it in its
// transaction (ADR 0012): the Library must be live (ErrLibraryNotFound) and,
// unless the repository is a Discard's, no Discard of it under way
// (ErrLibraryDiscarding); a Discard's repository writes only while one is.
// Removing a widget type touches no Scene: Projects see only the copies the
// releases they pin hold.
type WidgetTypeRepository interface {
	// NextID returns a new unique identifier for a widget type.
	NextID() id.ID[WidgetType]

	// Save stores a widget type and the events it recorded, in one
	// transaction: a widget type of the initial version is inserted, any
	// other replaces the stored one if the stored version is still its
	// version. The version is raised once per Save; the returned widget type
	// carries the new one and no events.
	// Returns ErrWidgetTypeNotFound if the record does not exist,
	// ErrWidgetTypeConflict if the stored version does not match,
	// ErrWidgetTypeNameTaken if another widget type of the Library has the
	// name, and the Library's errors above.
	Save(context.Context, WidgetType) (WidgetType, error)

	// FindByID returns a widget type by its identifier.
	// Returns ErrWidgetTypeNotFound if not found.
	FindByID(context.Context, id.ID[WidgetType]) (WidgetType, error)

	// Delete removes a widget type, read at the version it carries, and
	// stores the events it recorded (see WidgetType.Delete), in one
	// transaction.
	// Returns ErrWidgetTypeNotFound if the widget type does not exist,
	// ErrWidgetTypeConflict if the stored version does not match, and the
	// Library's errors above.
	Delete(context.Context, WidgetType) error

	// FindAll returns all widget types of the Library in creation order, so
	// the order is stable while widget types change. All are read at one
	// moment: the Draft as it was then.
	FindAll(context.Context) ([]WidgetType, error)
}

// WidgetTypeRepositories opens the WidgetTypeRepository of a Library's Draft.
type WidgetTypeRepositories interface {
	// ForLibrary returns the repository of the Library's widget types, whose
	// writes are refused while a Discard is under way.
	ForLibrary(id.ID[Library]) WidgetTypeRepository
	// ForDiscard returns the repository of the Library's widget types whose
	// writes are a Discard's: made only while one is under way.
	ForDiscard(id.ID[Library]) WidgetTypeRepository
}
