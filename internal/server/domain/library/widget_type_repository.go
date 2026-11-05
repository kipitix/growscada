package library

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

var (
	ErrWidgetTypeNotFound = errors.New("widget type not found")
	// ErrWidgetTypeNameTaken is returned by a write of the Draft when another
	// widget type of the same Library already has the name: within its Library
	// a widget type's name is its natural key.
	ErrWidgetTypeNameTaken = errors.New("widget type name already taken")
	// ErrInvalidWidgetType is returned by WidgetType.Update when the new
	// definition breaks a rule of the widget type as a whole (e.g. two
	// InputPorts share a name).
	ErrInvalidWidgetType = errors.New("invalid widget type")
)

// WidgetTypeRepository reads the WidgetTypes of the Draft of one Library:
// every read sees only the widget types of that Library, so one of another is
// not found. The writes are the Library Draft's (draft.Session, ADR 0016):
// they state its Version and write one DraftChange. Removing a widget type
// touches no Scene: Projects see only the copies the releases they pin hold.
type WidgetTypeRepository interface {
	// NextID returns a new unique identifier for a widget type.
	NextID() id.ID[WidgetType]

	// FindByID returns a widget type by its identifier.
	// Returns ErrWidgetTypeNotFound if not found.
	FindByID(context.Context, id.ID[WidgetType]) (WidgetType, error)

	// FindAll returns all widget types of the Library in creation order, so
	// the order is stable while widget types change. All are read at one
	// moment: the Draft as it was then.
	FindAll(context.Context) ([]WidgetType, error)
}

// WidgetTypeRepositories opens the WidgetTypeRepository of a Library's Draft.
type WidgetTypeRepositories interface {
	// ForLibrary returns the read repository of the Library's widget types.
	ForLibrary(id.ID[Library]) WidgetTypeRepository
}
