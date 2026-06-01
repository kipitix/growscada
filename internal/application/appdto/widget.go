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

// Widget is the application-layer DTO for widget instance data.
type Widget struct {
	ID              uuid.UUID
	Name            string
	X, Y            float64
	Z               int
	Width           int
	Height          int
	OriginX, OriginY float64
	RotationDegrees float64
	TransformMatrix TransformMatrix
	TypeID          uuid.UUID
	SceneID         uuid.UUID
	Labels          []string
	PortBindings    []PortBinding
	Version         int
}

// CreateWidgetInput holds the input data for creating a widget instance.
type CreateWidgetInput struct {
	Name            string
	X, Y            float64
	Z               int
	Width           int
	Height          int
	OriginX, OriginY float64
	RotationDegrees float64
	TypeID          uuid.UUID
	SceneID         uuid.UUID
	Labels          []string
	PortBindings    []PortBinding
}

// UpdateWidgetInput holds the input data for updating a widget instance.
// Version must match the current persisted version for optimistic locking.
type UpdateWidgetInput struct {
	ID              uuid.UUID
	Name            string
	X, Y            float64
	Z               int
	Width           int
	Height          int
	OriginX, OriginY float64
	RotationDegrees float64
	TypeID          uuid.UUID
	SceneID         uuid.UUID
	Labels          []string
	PortBindings    []PortBinding
	Version         int
}

// NewWidget creates a Widget DTO from the domain aggregate.
// The TransformationMatrix is computed on the fly from the domain object.
func NewWidget(w widget.Widget) Widget {
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
		SceneID:      w.SceneID().UUID(),
		Labels:       labels,
		PortBindings: portBindings,
		Version:      w.Version().Number(),
	}
}

// NewWidgetList creates a slice of Widget DTOs from domain aggregates.
func NewWidgetList(list []widget.Widget) []Widget {
	result := make([]Widget, len(list))
	for i, w := range list {
		result[i] = NewWidget(w)
	}
	return result
}
