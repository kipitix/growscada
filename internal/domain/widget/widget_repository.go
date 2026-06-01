package widget

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
)

var (
	ErrWidgetNotFound = errors.New("widget not found")
	// ErrWidgetConflict is returned by Save when the stored version does not match,
	// indicating a concurrent modification.
	ErrWidgetConflict = errors.New("widget version conflict")
	// ErrWidgetInvalidInput is returned when required fields are missing or invalid.
	ErrWidgetInvalidInput = errors.New("widget invalid input")
)

// WidgetRepository - repository interface for storing and managing widget instances.
type WidgetRepository interface {
	// NextID returns a new unique identifier for a widget.
	NextID() id.ID[Widget]

	// Save stores a widget in the repository.
	// Returns ErrWidgetNotFound if the record does not exist on update.
	// Returns ErrWidgetConflict if the stored version does not match.
	Save(context.Context, Widget) (Widget, error)

	// FindByID returns a widget by its identifier.
	// Returns ErrWidgetNotFound if not found.
	FindByID(context.Context, id.ID[Widget]) (Widget, error)

	// DeleteByID removes a widget by its identifier and returns it.
	// Returns ErrWidgetNotFound if the widget does not exist.
	DeleteByID(context.Context, id.ID[Widget]) (Widget, error)

	// FindAll returns all widgets across all scenes.
	FindAll(context.Context) ([]Widget, error)

	// FindBySceneID returns all widgets belonging to the given scene.
	FindBySceneID(context.Context, id.ID[scene.Scene]) ([]Widget, error)

	// FindByTypeID returns all widgets that use the given widget type.
	FindByTypeID(context.Context, id.ID[WidgetType]) ([]Widget, error)
}
