package library

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

var (
	ErrWidgetTypeNotFound = errors.New("widget type not found")
	// ErrWidgetTypeConflict is returned by Save when the stored version does not match,
	// indicating a concurrent modification.
	ErrWidgetTypeConflict = errors.New("widget type version conflict")
	// ErrWidgetTypeInUse is returned by DeleteByID when Widgets still use the
	// widget type: a Widget's WidgetType always exists.
	ErrWidgetTypeInUse = errors.New("widget type is used by widgets")
)

// WidgetTypeRepository - repository interface for storing and managing widget types.
type WidgetTypeRepository interface {
	// NextID returns a new unique identifier for a widget type.
	NextID() id.ID[WidgetType]

	// Save stores a widget type in the repository.
	// Returns ErrWidgetTypeNotFound if the record does not exist.
	// Returns ErrWidgetTypeConflict if the stored version does not match.
	Save(context.Context, WidgetType) (WidgetType, error)

	// FindByID returns a widget type by its identifier.
	// Returns ErrWidgetTypeNotFound if not found.
	FindByID(context.Context, id.ID[WidgetType]) (WidgetType, error)

	// DeleteByID removes a widget type by its identifier and returns it.
	// Returns ErrWidgetTypeNotFound if the widget type does not exist,
	// ErrWidgetTypeInUse if Widgets still use it.
	DeleteByID(context.Context, id.ID[WidgetType]) (WidgetType, error)

	// FindAll returns all widget types in creation order, so the order is
	// stable while widget types change.
	FindAll(context.Context) ([]WidgetType, error)
}
