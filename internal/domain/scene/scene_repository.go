package scene

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/version"
	"github.com/kipitix/growscada/internal/domain/widget"
)

var (
	ErrSceneNotFound = errors.New("scene not found")
	// ErrSceneConflict is returned by Save/widget operations when the stored
	// version does not match, indicating a concurrent modification. Since Widget
	// has no version of its own, this also guards widget mutations.
	ErrSceneConflict = errors.New("scene version conflict")
	// ErrSceneValidation is returned when input fails domain validation
	// (e.g. empty name, non-positive dimensions).
	ErrSceneValidation = errors.New("scene validation error")
)

// SceneRepository - repository interface for storing and managing scenes and
// their widgets. Widget, as an entity of the Scene aggregate, is only ever
// reached through this repository — there is no standalone WidgetRepository.
type SceneRepository interface {
	// NextID returns a new unique identifier for a scene.
	NextID() id.ID[Scene]

	// NextWidgetID returns a new unique identifier for a widget.
	NextWidgetID() id.ID[widget.Widget]

	// Save stores a scene's own fields (name, size, background). It does not
	// touch the scene's widgets.
	// Returns ErrSceneNotFound if the record does not exist on update.
	// Returns ErrSceneConflict if the stored version does not match.
	Save(context.Context, Scene) (Scene, error)

	// FindByID returns a scene, with its widgets, by its identifier.
	// Returns ErrSceneNotFound if not found.
	FindByID(context.Context, id.ID[Scene]) (Scene, error)

	// DeleteByID removes a scene and (via cascade) all its widgets, returning
	// the scene as it existed immediately before deletion, widgets included.
	// Returns ErrSceneNotFound if the scene does not exist.
	DeleteByID(context.Context, id.ID[Scene]) (Scene, error)

	// FindAll returns all scenes, with their widgets.
	FindAll(context.Context) ([]Scene, error)

	// FindWidgetsBySceneID returns all widgets belonging to the given scene,
	// without loading the rest of the scene.
	FindWidgetsBySceneID(context.Context, id.ID[Scene]) ([]widget.Widget, error)

	// FindWidgetByID returns a single widget belonging to the given scene.
	// Returns ErrSceneNotFound if the scene does not exist, widget.ErrWidgetNotFound
	// if the widget does not exist within it.
	FindWidgetByID(context.Context, id.ID[Scene], id.ID[widget.Widget]) (widget.Widget, error)

	// AddWidget inserts a new widget into the scene, checking expectedVersion
	// against the scene's current version and bumping it atomically.
	// Returns ErrSceneNotFound if the scene does not exist.
	// Returns ErrSceneConflict if expectedVersion does not match.
	AddWidget(ctx context.Context, sceneID id.ID[Scene], expectedVersion version.Version[Scene], w widget.Widget) (widget.Widget, version.Version[Scene], error)

	// UpdateWidget replaces an existing widget's fields, checking expectedVersion
	// against the scene's current version and bumping it atomically.
	// Returns ErrSceneNotFound if the scene does not exist.
	// Returns ErrSceneConflict if expectedVersion does not match.
	// Returns widget.ErrWidgetNotFound if the widget does not exist within the scene.
	UpdateWidget(ctx context.Context, sceneID id.ID[Scene], expectedVersion version.Version[Scene], w widget.Widget) (widget.Widget, version.Version[Scene], error)

	// DeleteWidget removes a widget from the scene and bumps the scene's
	// version unconditionally (no optimistic-lock check: deletion carries no
	// risk of silently overwriting someone else's newer content).
	// Returns ErrSceneNotFound if the scene does not exist.
	// Returns widget.ErrWidgetNotFound if the widget does not exist within the scene.
	DeleteWidget(ctx context.Context, sceneID id.ID[Scene], widgetID id.ID[widget.Widget]) (widget.Widget, version.Version[Scene], error)

	// FindWidgetsByTypeID returns every widget using the given widget type,
	// across all scenes, each tagged with the owning scene's id and current
	// version so the caller can update it back via UpdateWidget. Used only by
	// WidgetType maintenance (port binding cleanup on type update/delete) —
	// the one legitimate cross-scene widget query.
	FindWidgetsByTypeID(ctx context.Context, typeID id.ID[widget.WidgetType]) ([]WidgetInScene, error)
}

// WidgetInScene pairs a widget with the id and current version of the scene
// that owns it. Returned by FindWidgetsByTypeID, whose results span scenes.
type WidgetInScene struct {
	SceneID      id.ID[Scene]
	SceneVersion version.Version[Scene]
	Widget       widget.Widget
}
