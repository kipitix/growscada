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

// WidgetResponse is the HTTP DTO for representing a widget instance in API
// responses. Version is the Version of the Project's Draft, the same in every
// content entity of one response (ADR 0016).
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
	Version         int                     `json:"version"`
	Labels          []string                `json:"labels"`
	PortBindings    []PortBinding           `json:"port_bindings"`
}

// GetWidgetsResponse is the HTTP DTO for a list of widget instances.
type GetWidgetsResponse struct {
	Widgets []WidgetResponse `json:"widgets"`
}

// CreateWidgetRequest is the HTTP DTO for creating a widget instance.
// The owning scene is taken from the URL path. Version is the Version of the
// Draft the client saw (ADR 0016).
type CreateWidgetRequest struct {
	Name         string          `json:"name"`
	Position     PositionRequest `json:"position"`
	Size         SizeRequest     `json:"size"`
	Origin       OriginRequest   `json:"origin"`
	Rotation     RotationRequest `json:"rotation"`
	TypeID       uuid.UUID       `json:"type_id"`
	Version      int             `json:"version"`
	Labels       []string        `json:"labels"`
	PortBindings []PortBinding   `json:"port_bindings"`
}

// CreateWidgetResponse is the HTTP DTO for a widget creation response.
// Version is the Draft's Version after the write.
type CreateWidgetResponse struct {
	ID      uuid.UUID `json:"id"`
	Version int       `json:"version"`
}

// UpdateWidgetRequest is the HTTP DTO for updating a widget instance.
// Version must be the Draft's current Version (ADR 0016).
type UpdateWidgetRequest struct {
	Name         string          `json:"name"`
	Position     PositionRequest `json:"position"`
	Size         SizeRequest     `json:"size"`
	Origin       OriginRequest   `json:"origin"`
	Rotation     RotationRequest `json:"rotation"`
	TypeID       uuid.UUID       `json:"type_id"`
	Version      int             `json:"version"`
	Labels       []string        `json:"labels"`
	PortBindings []PortBinding   `json:"port_bindings"`
}

// UpdateWidgetResponse is the HTTP DTO for a widget update response.
type UpdateWidgetResponse struct {
	Version int `json:"version"`
}

// MoveWidgetsRequest is the HTTP DTO for moving several widgets of one scene at
// once, as one change of the Draft (ADR 0010). Version must be the Draft's
// current Version (ADR 0016).
type MoveWidgetsRequest struct {
	Version int                 `json:"version"`
	Moves   []WidgetMoveRequest `json:"moves"`
}

// WidgetMoveRequest is one widget of a MoveWidgetsRequest: the widget's ID and
// the position it takes. The layer (z) stays as stored.
type WidgetMoveRequest struct {
	WidgetID uuid.UUID `json:"widget_id"`
	X        float64   `json:"x"`
	Y        float64   `json:"y"`
}

// MoveWidgetsResponse is the answer to a move: the Version of the Draft after
// the change.
type MoveWidgetsResponse struct {
	Version int `json:"version"`
}
