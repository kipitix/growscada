package apiv0

import (
	"github.com/google/uuid"
)

// SceneResponse is the HTTP DTO for representing a scene in API responses.
type SceneResponse struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Width          int       `json:"width"`
	Height         int       `json:"height"`
	BackgroundHTML string    `json:"background_html"`
	Version        int       `json:"version"`
}

// GetScenesResponse is the HTTP DTO for a list of scenes.
type GetScenesResponse struct {
	Scenes []SceneResponse `json:"scenes"`
}

// CreateSceneRequest is the HTTP DTO for creating a scene.
type CreateSceneRequest struct {
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	BackgroundHTML string `json:"background_html"`
}

// CreateSceneResponse is the HTTP DTO for a scene creation response.
type CreateSceneResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateSceneRequest is the HTTP DTO for updating a scene.
// Version must match the current persisted version for optimistic locking.
type UpdateSceneRequest struct {
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	BackgroundHTML string `json:"background_html"`
	Version        int    `json:"version"`
}

// UpdateSceneResponse is the HTTP DTO for a scene update response.
type UpdateSceneResponse struct {
	Version int `json:"version"`
}
