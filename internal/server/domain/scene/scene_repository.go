package scene

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

var (
	ErrSceneNotFound = errors.New("scene not found")
	// ErrSceneConflict is returned by the Scene's change methods when the
	// expected version is not the current one (an edit conflict), and by Save
	// when the stored version changed after the scene was read (a write race).
	// Since Widget has no version of its own, this also guards widget changes.
	ErrSceneConflict = errors.New("scene version conflict")
)

// SceneRepository - repository interface for storing and managing scenes
// together with their widgets. Widget, as an entity of the Scene aggregate,
// is only ever read and written as part of its Scene — there is no
// standalone WidgetRepository and no per-widget method here.
type SceneRepository interface {
	// NextID returns a new unique identifier for a scene.
	NextID() id.ID[Scene]

	// NextWidgetID returns a new unique identifier for a widget.
	NextWidgetID() id.ID[Widget]

	// FindByID returns a scene, with its widgets, by its identifier.
	// Returns ErrSceneNotFound if not found.
	FindByID(context.Context, id.ID[Scene]) (Scene, error)

	// FindAll returns all scenes, with their widgets, in creation order, so
	// the order is stable while scenes change (scene tabs do not jump).
	FindAll(context.Context) ([]Scene, error)

	// FindByWidgetTypeID returns, in creation order, every scene with at
	// least one widget of the given WidgetType, each with all its widgets.
	FindByWidgetTypeID(context.Context, id.ID[library.WidgetType]) ([]Scene, error)

	// Save stores a scene with its widgets: a scene of the initial version is
	// inserted, any other replaces the stored one if the stored version is
	// still the scene's version. Widgets the scene no longer holds are
	// removed. The version is raised once per Save; the returned scene
	// carries the new one and its widgets as stored.
	// Returns ErrSceneNotFound if the scene does not exist on update.
	// Returns ErrSceneConflict if the stored version does not match.
	// Returns library.ErrWidgetTypeNotFound if a widget's type does not exist.
	Save(context.Context, Scene) (Scene, error)

	// DeleteByID removes a scene and (via cascade) all its widgets, returning
	// the scene as it existed immediately before deletion, widgets included.
	// Returns ErrSceneNotFound if the scene does not exist.
	DeleteByID(context.Context, id.ID[Scene]) (Scene, error)
}
