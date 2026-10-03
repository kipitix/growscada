package apiv0

import (
	"github.com/google/uuid"
)

// --- Sub-DTOs for geometric properties ---

// PositionRequest is the HTTP DTO for providing widget position.
type PositionRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z int     `json:"z"`
}

// PositionResponse is the HTTP DTO for returning widget position.
type PositionResponse struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z int     `json:"z"`
}

// SizeRequest is the HTTP DTO for providing widget dimensions.
type SizeRequest struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// SizeResponse is the HTTP DTO for returning widget dimensions.
type SizeResponse struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// OriginRequest is the HTTP DTO for providing the widget's anchor point.
// X and Y must be in [0, 1]: (0.5, 0.5) is the center.
type OriginRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// OriginResponse is the HTTP DTO for returning the widget's anchor point.
type OriginResponse struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// RotationRequest is the HTTP DTO for providing the widget's rotation angle.
type RotationRequest struct {
	Degrees float64 `json:"degrees"`
}

// RotationResponse is the HTTP DTO for returning the widget's rotation angle.
type RotationResponse struct {
	Degrees float64 `json:"degrees"`
}

// TransformMatrixResponse is the HTTP DTO for the computed 2D affine matrix.
// All fields are read-only — the matrix is always derived from Position, Origin,
// Rotation and Size; it is never accepted as input.
type TransformMatrixResponse struct {
	A   float64 `json:"a"`
	B   float64 `json:"b"`
	C   float64 `json:"c"`
	D   float64 `json:"d"`
	E   float64 `json:"e"`
	F   float64 `json:"f"`
	CSS string  `json:"css"`
}

// PortBinding links one of a widget's input ports to the tag that feeds it.
type PortBinding struct {
	PortName string    `json:"port_name"`
	TagID    uuid.UUID `json:"tag_id"`
}

// --- Widget request / response types ---

// WidgetResponse is the HTTP DTO for representing a widget instance in API responses.
// Widget has no version of its own — SceneVersion is the owning scene's current
// version, the sole optimistic-lock boundary for the scene and all its widgets.
type WidgetResponse struct {
	ID              uuid.UUID               `json:"id"`
	Name            string                  `json:"name"`
	Position        PositionResponse        `json:"position"`
	Size            SizeResponse            `json:"size"`
	Origin          OriginResponse          `json:"origin"`
	Rotation        RotationResponse        `json:"rotation"`
	TransformMatrix TransformMatrixResponse `json:"transform_matrix"`
	TypeID          uuid.UUID               `json:"type_id"`
	SceneID         uuid.UUID               `json:"scene_id"`
	SceneVersion    int                     `json:"scene_version"`
	Labels          []string                `json:"labels"`
	PortBindings    []PortBinding           `json:"port_bindings"`
}

// GetWidgetsResponse is the HTTP DTO for a list of widget instances.
type GetWidgetsResponse struct {
	Widgets []WidgetResponse `json:"widgets"`
}

// CreateWidgetRequest is the HTTP DTO for creating a widget instance.
// The owning scene is taken from the URL path. SceneVersion must equal the
// scene's current persisted version for optimistic locking.
type CreateWidgetRequest struct {
	Name         string          `json:"name"`
	Position     PositionRequest `json:"position"`
	Size         SizeRequest     `json:"size"`
	Origin       OriginRequest   `json:"origin"`
	Rotation     RotationRequest `json:"rotation"`
	TypeID       uuid.UUID       `json:"type_id"`
	SceneVersion int             `json:"scene_version"`
	Labels       []string        `json:"labels"`
	PortBindings []PortBinding   `json:"port_bindings"`
}

// CreateWidgetResponse is the HTTP DTO for a widget creation response.
type CreateWidgetResponse struct {
	ID           uuid.UUID `json:"id"`
	SceneVersion int       `json:"scene_version"`
}

// UpdateWidgetRequest is the HTTP DTO for updating a widget instance.
// SceneVersion must equal the owning scene's current persisted version for
// optimistic locking.
type UpdateWidgetRequest struct {
	Name         string          `json:"name"`
	Position     PositionRequest `json:"position"`
	Size         SizeRequest     `json:"size"`
	Origin       OriginRequest   `json:"origin"`
	Rotation     RotationRequest `json:"rotation"`
	TypeID       uuid.UUID       `json:"type_id"`
	SceneVersion int             `json:"scene_version"`
	Labels       []string        `json:"labels"`
	PortBindings []PortBinding   `json:"port_bindings"`
}

// UpdateWidgetResponse is the HTTP DTO for a widget update response.
type UpdateWidgetResponse struct {
	SceneVersion int `json:"scene_version"`
}
