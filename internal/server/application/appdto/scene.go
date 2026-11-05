package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/domain/scene"
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

// SceneInput holds the fields of a scene a client creates or updates. The
// scene's ID and the version an update expects are passed separately.
type SceneInput struct {
	Name           string
	Width          int
	Height         int
	BackgroundHTML string
}

// NewScene creates a Scene DTO from the domain aggregate; version is the
// Version of its Project's Draft, which the DTO carries.
func NewScene(s scene.Scene, version int) Scene {
	return Scene{
		ID:             s.ID().UUID(),
		Name:           s.Name().String(),
		Width:          s.Size().Width(),
		Height:         s.Size().Height(),
		BackgroundHTML: s.BackgroundHTML().Content(),
		Version:        version,
	}
}

// NewSceneList creates a slice of Scene DTOs from domain aggregates.
func NewSceneList(list []scene.Scene, version int) []Scene {
	result := make([]Scene, len(list))
	for i, s := range list {
		result[i] = NewScene(s, version)
	}
	return result
}
