package widget

import (
	"context"
	"errors"
)

var (
	ErrWidgetTypeNotFound = errors.New("widget type not found")
	// ErrWidgetTypeConflict is returned by Save when the stored version does not match,
	// indicating a concurrent modification.
	ErrWidgetTypeConflict = errors.New("widget type version conflict")
)

// WidgetTypeRepository - repository interface for storing and managing widget types.
type WidgetTypeRepository interface {
	// NextID returns a new unique identifier for a widget type.
	NextID() WidgetTypeID

	// Save stores a widget type in the repository.
	// Returns ErrWidgetTypeNotFound if the record does not exist.
	// Returns ErrWidgetTypeConflict if the stored version does not match.
	Save(context.Context, WidgetType) (WidgetType, error)

	// FindByID returns a widget type by its identifier.
	// Returns ErrWidgetTypeNotFound if not found.
	FindByID(context.Context, WidgetTypeID) (WidgetType, error)

	// DeleteByID removes a widget type by its identifier and returns it.
	// Returns ErrWidgetTypeNotFound if the widget type does not exist.
	DeleteByID(context.Context, WidgetTypeID) (WidgetType, error)

	// FindAll returns all widget types.
	FindAll(context.Context) ([]WidgetType, error)
}
