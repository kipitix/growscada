package restdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
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

// PortBindingDTO is the HTTP DTO for a widget port binding.
type PortBindingDTO struct {
	PortName string    `json:"port_name"`
	TagID    uuid.UUID `json:"tag_id"`
}

// --- Widget request / response types ---

// WidgetResponse is the HTTP DTO for representing a widget instance in API responses.
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
	Labels          []string                `json:"labels"`
	PortBindings    []PortBindingDTO        `json:"port_bindings"`
	Version         int                     `json:"version"`
}

// GetWidgetsResponse is the HTTP DTO for a list of widget instances.
type GetWidgetsResponse struct {
	Widgets []WidgetResponse `json:"widgets"`
}

// CreateWidgetRequest is the HTTP DTO for creating a widget instance.
type CreateWidgetRequest struct {
	Name         string           `json:"name"`
	Position     PositionRequest  `json:"position"`
	Size         SizeRequest      `json:"size"`
	Origin       OriginRequest    `json:"origin"`
	Rotation     RotationRequest  `json:"rotation"`
	TypeID       uuid.UUID        `json:"type_id"`
	SceneID      uuid.UUID        `json:"scene_id"`
	Labels       []string         `json:"labels"`
	PortBindings []PortBindingDTO `json:"port_bindings"`
}

// CreateWidgetResponse is the HTTP DTO for a widget creation response.
type CreateWidgetResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateWidgetRequest is the HTTP DTO for updating a widget instance.
// Version must equal the current persisted version for optimistic locking.
type UpdateWidgetRequest struct {
	Name         string           `json:"name"`
	Position     PositionRequest  `json:"position"`
	Size         SizeRequest      `json:"size"`
	Origin       OriginRequest    `json:"origin"`
	Rotation     RotationRequest  `json:"rotation"`
	TypeID       uuid.UUID        `json:"type_id"`
	SceneID      uuid.UUID        `json:"scene_id"`
	Labels       []string         `json:"labels"`
	PortBindings []PortBindingDTO `json:"port_bindings"`
	Version      int              `json:"version"`
}

// UpdateWidgetResponse is the HTTP DTO for a widget update response.
type UpdateWidgetResponse struct {
	Version int `json:"version"`
}

// --- Mapping functions ---

func portBindingDTOsToAppDTOs(bindings []PortBindingDTO) []appdto.PortBinding {
	result := make([]appdto.PortBinding, len(bindings))
	for i, b := range bindings {
		result[i] = appdto.PortBinding{PortName: b.PortName, TagID: b.TagID}
	}
	return result
}

func appPortBindingDTOsToRest(bindings []appdto.PortBinding) []PortBindingDTO {
	result := make([]PortBindingDTO, len(bindings))
	for i, b := range bindings {
		result[i] = PortBindingDTO{PortName: b.PortName, TagID: b.TagID}
	}
	return result
}

func NewWidgetResponse(w appdto.Widget) WidgetResponse {
	labels := w.Labels
	if labels == nil {
		labels = []string{}
	}
	portBindings := appPortBindingDTOsToRest(w.PortBindings)
	return WidgetResponse{
		ID:   w.ID,
		Name: w.Name,
		Position: PositionResponse{
			X: w.X,
			Y: w.Y,
			Z: w.Z,
		},
		Size: SizeResponse{
			Width:  w.Width,
			Height: w.Height,
		},
		Origin: OriginResponse{
			X: w.OriginX,
			Y: w.OriginY,
		},
		Rotation: RotationResponse{
			Degrees: w.RotationDegrees,
		},
		TransformMatrix: TransformMatrixResponse{
			A:   w.TransformMatrix.A,
			B:   w.TransformMatrix.B,
			C:   w.TransformMatrix.C,
			D:   w.TransformMatrix.D,
			E:   w.TransformMatrix.E,
			F:   w.TransformMatrix.F,
			CSS: w.TransformMatrix.CSS,
		},
		TypeID:       w.TypeID,
		SceneID:      w.SceneID,
		Labels:       labels,
		PortBindings: portBindings,
		Version:      w.Version,
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
		Name:            r.Name,
		X:               r.Position.X,
		Y:               r.Position.Y,
		Z:               r.Position.Z,
		Width:           r.Size.Width,
		Height:          r.Size.Height,
		OriginX:         r.Origin.X,
		OriginY:         r.Origin.Y,
		RotationDegrees: r.Rotation.Degrees,
		TypeID:          r.TypeID,
		SceneID:         r.SceneID,
		Labels:          r.Labels,
		PortBindings:    portBindingDTOsToAppDTOs(r.PortBindings),
	}
}

func NewUpdateWidgetInput(r UpdateWidgetRequest, widgetID uuid.UUID) appdto.UpdateWidgetInput {
	return appdto.UpdateWidgetInput{
		ID:              widgetID,
		Name:            r.Name,
		X:               r.Position.X,
		Y:               r.Position.Y,
		Z:               r.Position.Z,
		Width:           r.Size.Width,
		Height:          r.Size.Height,
		OriginX:         r.Origin.X,
		OriginY:         r.Origin.Y,
		RotationDegrees: r.Rotation.Degrees,
		TypeID:          r.TypeID,
		SceneID:         r.SceneID,
		Labels:          r.Labels,
		PortBindings:    portBindingDTOsToAppDTOs(r.PortBindings),
		Version:         r.Version,
	}
}
