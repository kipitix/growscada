package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// TransformMatrix is the application-layer representation of the computed
// 2D affine transformation matrix (CSS matrix(a,b,c,d,e,f) form).
type TransformMatrix struct {
	A, B, C, D, E, F float64
	CSS              string
}

// PortBinding is the application-layer DTO for a widget port binding.
type PortBinding struct {
	PortName string
	TagID    uuid.UUID
}

// Widget is the application-layer DTO for widget instance data. Version is
// the Version of the Project's Draft, the same in every entity of one
// response (ADR 0016): a Widget is an entity of the Draft like its Scene.
type Widget struct {
	ID               uuid.UUID
	Name             string
	X, Y             float64
	Z                int
	Width            int
	Height           int
	OriginX, OriginY float64
	RotationDegrees  float64
	TransformMatrix  TransformMatrix
	TypeID           uuid.UUID
	SceneID          uuid.UUID
	Version          int
	Labels           []string
	PortBindings     []PortBinding
}

// WidgetInput holds the fields of a widget instance a client creates or
// updates. The owning scene, the widget's ID and the scene version the write
// expects are passed separately.
type WidgetInput struct {
	Name             string
	X, Y             float64
	Z                int
	Width            int
	Height           int
	OriginX, OriginY float64
	RotationDegrees  float64
	TypeID           uuid.UUID
	Labels           []string
	PortBindings     []PortBinding
}

// WidgetMove is one widget's new place on its scene's canvas: the ID of the
// entity and the x and y of the position it takes. The layer (z), the size,
// the origin, the rotation, the type, the labels and the bindings stay as
// stored (ADR 0010).
type WidgetMove struct {
	WidgetID uuid.UUID
	X, Y     float64
}

// NewWidget creates a Widget DTO from the domain entity. sceneID and
// sceneVersion come from the owning Scene, since Widget itself carries neither.
func NewWidget(w scene.Widget, sceneID uuid.UUID, sceneVersion int) Widget {
	domainBindings := w.PortBindings()
	portBindings := make([]PortBinding, len(domainBindings))
	for i, b := range domainBindings {
		portBindings[i] = PortBinding{
			PortName: b.PortName().String(),
			TagID:    b.TagID().UUID(),
		}
	}

	labels := make([]string, len(w.Labels()))
	copy(labels, w.Labels())

	m := w.TransformationMatrix()

	return Widget{
		ID:              w.ID().UUID(),
		Name:            w.Name().String(),
		X:               w.Position().X(),
		Y:               w.Position().Y(),
		Z:               w.Position().Z(),
		Width:           w.Size().Width(),
		Height:          w.Size().Height(),
		OriginX:         w.Origin().X(),
		OriginY:         w.Origin().Y(),
		RotationDegrees: w.Rotation().Degrees(),
		TransformMatrix: TransformMatrix{
			A:   m.A(),
			B:   m.B(),
			C:   m.C(),
			D:   m.D(),
			E:   m.E(),
			F:   m.F(),
			CSS: m.CSS(),
		},
		TypeID:       w.TypeID().UUID(),
		SceneID:      sceneID,
		Version:      sceneVersion,
		Labels:       labels,
		PortBindings: portBindings,
	}
}

// NewWidgetList creates a slice of Widget DTOs from domain entities that all
// belong to the same scene.
func NewWidgetList(list []scene.Widget, sceneID uuid.UUID, sceneVersion int) []Widget {
	result := make([]Widget, len(list))
	for i, w := range list {
		result[i] = NewWidget(w, sceneID, sceneVersion)
	}
	return result
}
