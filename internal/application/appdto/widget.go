package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/widget"
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

// Widget is the application-layer DTO for widget instance data. SceneVersion
// is the version of the owning scene: Widget has no version of its own, since
// it is an entity of the Scene aggregate.
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
	SceneVersion     int
	Labels           []string
	PortBindings     []PortBinding
}

// CreateWidgetInput holds the input data for creating a widget instance.
// The owning scene is identified separately (it is part of the URL path, not
// the body); SceneVersion must match that scene's current persisted version
// for optimistic locking.
type CreateWidgetInput struct {
	Name             string
	X, Y             float64
	Z                int
	Width            int
	Height           int
	OriginX, OriginY float64
	RotationDegrees  float64
	TypeID           uuid.UUID
	SceneVersion     int
	Labels           []string
	PortBindings     []PortBinding
}

// UpdateWidgetInput holds the input data for updating a widget instance.
// SceneVersion must match the owning scene's current persisted version for
// optimistic locking.
type UpdateWidgetInput struct {
	ID               uuid.UUID
	Name             string
	X, Y             float64
	Z                int
	Width            int
	Height           int
	OriginX, OriginY float64
	RotationDegrees  float64
	TypeID           uuid.UUID
	SceneVersion     int
	Labels           []string
	PortBindings     []PortBinding
}

// NewWidget creates a Widget DTO from the domain entity. sceneID and
// sceneVersion come from the owning Scene, since Widget itself carries neither.
func NewWidget(w widget.Widget, sceneID uuid.UUID, sceneVersion int) Widget {
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
		SceneVersion: sceneVersion,
		Labels:       labels,
		PortBindings: portBindings,
	}
}

// NewWidgetList creates a slice of Widget DTOs from domain entities that all
// belong to the same scene.
func NewWidgetList(list []widget.Widget, sceneID uuid.UUID, sceneVersion int) []Widget {
	result := make([]Widget, len(list))
	for i, w := range list {
		result[i] = NewWidget(w, sceneID, sceneVersion)
	}
	return result
}
