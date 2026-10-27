package scene

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

var (
	ErrSceneNotFound = errors.New("scene not found")
	// ErrSceneConflict is returned by the Scene's change methods when the
	// expected version is not the current one (an edit conflict), and by Save
	// and Delete when the stored version changed after the scene was read
	// (a write race).
	// Since Widget has no version of its own, this also guards widget changes.
	ErrSceneConflict = errors.New("scene version conflict")
)

// SceneRepository - repository interface for storing and managing the scenes
// of one Project (ADR 0006) together with their widgets: every read sees only
// the scenes of that Project, so a scene of another one is not found, and
// saving or deleting a scene of another Project is a programming error.
// Widget, as an entity of the Scene aggregate, is only ever read and written
// as part of its Scene — there is no standalone WidgetRepository and no
// per-widget method here.
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

	// FindByIDInLiveProject and FindAllInLiveProject are FindByID and FindAll
	// that also check, in the same query, that the Project is live and of the
	// Organization: for the lists the UI reloads on every event (the Scenes,
	// the Widgets of one), which then need no read of the Project of their
	// own. A Project not one of those has no scenes: ErrSceneNotFound, an
	// empty list, which the caller tells apart from a missing scene or an
	// empty Project.
	FindByIDInLiveProject(context.Context, id.ID[Scene], id.ID[organization.Organization]) (Scene, error)
	FindAllInLiveProject(context.Context, id.ID[organization.Organization]) ([]Scene, error)

	// FindByTagID returns, in creation order, every scene with at least one
	// widget binding a port to the given Tag, each with all its widgets.
	FindByTagID(context.Context, id.ID[tag.Tag]) ([]Scene, error)

	// Save stores a scene with its widgets and the events it recorded, in one
	// transaction: a scene of the initial version is
	// inserted, any other replaces the stored one if the stored version is
	// still the scene's version. Widgets the scene no longer holds are
	// removed. The version is raised once per Save; the returned scene
	// carries the new one, its widgets as stored and no events.
	// Returns ErrSceneNotFound if the scene does not exist on update.
	// Returns ErrSceneConflict if the stored version does not match.
	// Returns library.ErrWidgetTypeNotFound if a widget added, or whose type
	// changed, is of a WidgetType no Library Release the Project pins holds.
	Save(context.Context, Scene) (Scene, error)

	// Delete removes a scene, read at the version it carries, and (via
	// cascade) all its widgets, and stores the events it recorded (see
	// Scene.Delete), in one transaction. The CAS makes the widgets removed
	// the ones the scene holds, each with its WidgetDeletedEvent.
	// Returns ErrSceneNotFound if the scene does not exist.
	// Returns ErrSceneConflict if the stored version does not match.
	Delete(context.Context, Scene) error
}
