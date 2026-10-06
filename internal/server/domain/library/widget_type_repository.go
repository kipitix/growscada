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
	// ErrWidgetTypeInUse is returned by Delete when Widgets still use the
	// widget type: a Widget's WidgetType always exists.
	ErrWidgetTypeInUse = errors.New("widget type is used by widgets")
	// ErrInvalidWidgetType is returned by WidgetType.Update when the new
	// definition breaks a rule of the widget type as a whole (e.g. two
	// InputPorts share a name).
	ErrInvalidWidgetType = errors.New("invalid widget type")
)

// WidgetTypeRepository - repository interface for storing and managing widget types.
type WidgetTypeRepository interface {
	// NextID returns a new unique identifier for a widget type.
	NextID() id.ID[WidgetType]

	// Save stores a widget type and the events it recorded, in one
	// transaction: a widget type of the initial version is inserted, any other
	// replaces the stored one if the stored version is still its version. The
	// version is raised once per Save; the returned widget type carries the
	// new one and no events.
	// Returns ErrWidgetTypeNotFound if the record does not exist.
	// Returns ErrWidgetTypeConflict if the stored version does not match.
	Save(context.Context, WidgetType) (WidgetType, error)

	// FindByID returns a widget type by its identifier.
	// Returns ErrWidgetTypeNotFound if not found.
	FindByID(context.Context, id.ID[WidgetType]) (WidgetType, error)

	// Delete removes a widget type, read at the version it carries, and
	// stores the events it recorded (see WidgetType.Delete), in one
	// transaction.
	// Returns ErrWidgetTypeNotFound if the widget type does not exist,
	// ErrWidgetTypeConflict if the stored version does not match,
	// ErrWidgetTypeInUse if Widgets still use it.
	Delete(context.Context, WidgetType) error

	// FindAll returns all widget types in creation order, so the order is
	// stable while widget types change.
	FindAll(context.Context) ([]WidgetType, error)
}
