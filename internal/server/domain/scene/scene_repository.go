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
)

// SceneRepository reads the scenes of one Project (ADR 0006) together with
// their widgets: every read sees only the scenes of that Project. A Scene is
// an entity of the Project's Draft (ADR 0016): its writes go through the
// Draft's repository, never through here.
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
}
