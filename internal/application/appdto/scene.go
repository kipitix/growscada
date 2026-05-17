package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/scene"
)

// Scene is the application-layer DTO for scene data.
type Scene struct {
	ID             uuid.UUID
	Name           string
	Width          int
	Height         int
	BackgroundHTML string
	Version        int
}

// CreateSceneInput holds the input data for creating a scene.
type CreateSceneInput struct {
	Name           string
	Width          int
	Height         int
	BackgroundHTML string
}

// UpdateSceneInput holds the input data for updating a scene.
type UpdateSceneInput struct {
	ID             uuid.UUID
	Name           string
	Width          int
	Height         int
	BackgroundHTML string
}

// NewScene creates a Scene DTO from the domain aggregate.
func NewScene(s scene.Scene) Scene {
	return Scene{
		ID:             s.ID().UUID(),
		Name:           s.Name().String(),
		Width:          s.Size().Width(),
		Height:         s.Size().Height(),
		BackgroundHTML: s.BackgroundHTML().Content(),
		Version:        s.Version().Number(),
	}
}

// NewSceneList creates a slice of Scene DTOs from domain aggregates.
func NewSceneList(list []scene.Scene) []Scene {
	result := make([]Scene, len(list))
	for i, s := range list {
		result[i] = NewScene(s)
	}
	return result
}
