package restdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
)

// CoordinatesRequest is the HTTP DTO for providing widget coordinates.
type CoordinatesRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// CoordinatesResponse is the HTTP DTO for returning widget coordinates.
type CoordinatesResponse struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// WidgetResponse is the HTTP DTO for representing a widget instance in API responses.
type WidgetResponse struct {
	ID          uuid.UUID           `json:"id"`
	Name        string              `json:"name"`
	Coordinates CoordinatesResponse `json:"coordinates"`
	Width       int                 `json:"width"`
	Height      int                 `json:"height"`
	TypeID      uuid.UUID           `json:"type_id"`
	SceneID     uuid.UUID           `json:"scene_id"`
	Labels      []string            `json:"labels"`
	TagIDs      []uuid.UUID         `json:"tag_ids"`
	Version     int                 `json:"version"`
}

// GetWidgetsResponse is the HTTP DTO for a list of widget instances.
type GetWidgetsResponse struct {
	Widgets []WidgetResponse `json:"widgets"`
}

// CreateWidgetRequest is the HTTP DTO for creating a widget instance.
type CreateWidgetRequest struct {
	Name        string             `json:"name"`
	Coordinates CoordinatesRequest `json:"coordinates"`
	Width       int                `json:"width"`
	Height      int                `json:"height"`
	TypeID      uuid.UUID          `json:"type_id"`
	SceneID     uuid.UUID          `json:"scene_id"`
	Labels      []string           `json:"labels"`
	TagIDs      []uuid.UUID        `json:"tag_ids"`
}

// CreateWidgetResponse is the HTTP DTO for a widget creation response.
type CreateWidgetResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateWidgetRequest is the HTTP DTO for updating a widget instance.
type UpdateWidgetRequest struct {
	Name        string             `json:"name"`
	Coordinates CoordinatesRequest `json:"coordinates"`
	Width       int                `json:"width"`
	Height      int                `json:"height"`
	TypeID      uuid.UUID          `json:"type_id"`
	SceneID     uuid.UUID          `json:"scene_id"`
	Labels      []string           `json:"labels"`
	TagIDs      []uuid.UUID        `json:"tag_ids"`
}

// UpdateWidgetResponse is the HTTP DTO for a widget update response.
type UpdateWidgetResponse struct {
	Version int `json:"version"`
}

func NewWidgetResponse(w appdto.Widget) WidgetResponse {
	labels := w.Labels
	if labels == nil {
		labels = []string{}
	}
	tagIDs := w.TagIDs
	if tagIDs == nil {
		tagIDs = []uuid.UUID{}
	}
	return WidgetResponse{
		ID:   w.ID,
		Name: w.Name,
		Coordinates: CoordinatesResponse{
			X: w.X,
			Y: w.Y,
			Z: w.Z,
		},
		Width:   w.Width,
		Height:  w.Height,
		TypeID:  w.TypeID,
		SceneID: w.SceneID,
		Labels:  labels,
		TagIDs:  tagIDs,
		Version: w.Version,
	}
}

func NewGetWidgetsResponse(list []appdto.Widget) GetWidgetsResponse {
	items := make([]WidgetResponse, len(list))
	for i, w := range list {
		items[i] = NewWidgetResponse(w)
	}
	return GetWidgetsResponse{Widgets: items}
}

func NewCreateWidgetResponse(w appdto.Widget) CreateWidgetResponse {
	return CreateWidgetResponse{ID: w.ID}
}

func NewUpdateWidgetResponse(w appdto.Widget) UpdateWidgetResponse {
	return UpdateWidgetResponse{Version: w.Version}
}

func NewCreateWidgetInput(r CreateWidgetRequest) appdto.CreateWidgetInput {
	return appdto.CreateWidgetInput{
		Name:    r.Name,
		X:       r.Coordinates.X,
		Y:       r.Coordinates.Y,
		Z:       r.Coordinates.Z,
		Width:   r.Width,
		Height:  r.Height,
		TypeID:  r.TypeID,
		SceneID: r.SceneID,
		Labels:  r.Labels,
		TagIDs:  r.TagIDs,
	}
}

func NewUpdateWidgetInput(r UpdateWidgetRequest, widgetID uuid.UUID) appdto.UpdateWidgetInput {
	return appdto.UpdateWidgetInput{
		ID:      widgetID,
		Name:    r.Name,
		X:       r.Coordinates.X,
		Y:       r.Coordinates.Y,
		Z:       r.Coordinates.Z,
		Width:   r.Width,
		Height:  r.Height,
		TypeID:  r.TypeID,
		SceneID: r.SceneID,
		Labels:  r.Labels,
		TagIDs:  r.TagIDs,
	}
}
