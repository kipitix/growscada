package scene

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/domain/id"
)

var (
	ErrSceneNotFound = errors.New("scene not found")
	// ErrSceneConflict is returned by Save when the stored version does not match,
	// indicating a concurrent modification.
	ErrSceneConflict = errors.New("scene version conflict")
)

// SceneRepository - repository interface for storing and managing scenes.
type SceneRepository interface {
	// NextID returns a new unique identifier for a scene.
	NextID() id.ID[Scene]

	// Save stores a scene in the repository.
	// Returns ErrSceneNotFound if the record does not exist on update.
	// Returns ErrSceneConflict if the stored version does not match.
	Save(context.Context, Scene) (Scene, error)

	// FindByID returns a scene by its identifier.
	// Returns ErrSceneNotFound if not found.
	FindByID(context.Context, id.ID[Scene]) (Scene, error)

	// DeleteByID removes a scene by its identifier and returns it.
	// Returns ErrSceneNotFound if the scene does not exist.
	DeleteByID(context.Context, id.ID[Scene]) (Scene, error)

	// FindAll returns all scenes.
	FindAll(context.Context) ([]Scene, error)
}
