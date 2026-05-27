package restdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
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

func NewSceneResponse(s appdto.Scene) SceneResponse {
	return SceneResponse{
		ID:             s.ID,
		Name:           s.Name,
		Width:          s.Width,
		Height:         s.Height,
		BackgroundHTML: s.BackgroundHTML,
		Version:        s.Version,
	}
}

func NewGetScenesResponse(list []appdto.Scene) GetScenesResponse {
	items := make([]SceneResponse, len(list))
	for i, s := range list {
		items[i] = NewSceneResponse(s)
	}
	return GetScenesResponse{Scenes: items}
}

func NewCreateSceneResponse(s appdto.Scene) CreateSceneResponse {
	return CreateSceneResponse{ID: s.ID}
}

func NewUpdateSceneResponse(s appdto.Scene) UpdateSceneResponse {
	return UpdateSceneResponse{Version: s.Version}
}

func NewCreateSceneInput(r CreateSceneRequest) appdto.CreateSceneInput {
	return appdto.CreateSceneInput{
		Name:           r.Name,
		Width:          r.Width,
		Height:         r.Height,
		BackgroundHTML: r.BackgroundHTML,
	}
}

func NewUpdateSceneInput(r UpdateSceneRequest, sceneID uuid.UUID) appdto.UpdateSceneInput {
	return appdto.UpdateSceneInput{
		ID:             sceneID,
		Name:           r.Name,
		Width:          r.Width,
		Height:         r.Height,
		BackgroundHTML: r.BackgroundHTML,
		Version:        r.Version,
	}
}
