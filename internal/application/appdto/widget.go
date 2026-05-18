package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// Widget is the application-layer DTO for widget instance data.
type Widget struct {
	ID      uuid.UUID
	Name    string
	X, Y, Z float64
	Width   int
	Height  int
	TypeID  uuid.UUID
	SceneID uuid.UUID
	Labels  []string
	TagIDs  []uuid.UUID
	Version int
}

// CreateWidgetInput holds the input data for creating a widget instance.
type CreateWidgetInput struct {
	Name    string
	X, Y, Z float64
	Width   int
	Height  int
	TypeID  uuid.UUID
	SceneID uuid.UUID
	Labels  []string
	TagIDs  []uuid.UUID
}

// UpdateWidgetInput holds the input data for updating a widget instance.
type UpdateWidgetInput struct {
	ID      uuid.UUID
	Name    string
	X, Y, Z float64
	Width   int
	Height  int
	TypeID  uuid.UUID
	SceneID uuid.UUID
	Labels  []string
	TagIDs  []uuid.UUID
}

// NewWidget creates a Widget DTO from the domain aggregate.
func NewWidget(w widget.Widget) Widget {
	tagIDs := make([]uuid.UUID, len(w.TagIDs()))
	for i, tid := range w.TagIDs() {
		tagIDs[i] = tid.UUID()
	}
	labels := make([]string, len(w.Labels()))
	copy(labels, w.Labels())
	return Widget{
		ID:      w.ID().UUID(),
		Name:    w.Name().String(),
		X:       w.Coordinates().X(),
		Y:       w.Coordinates().Y(),
		Z:       w.Coordinates().Z(),
		Width:   w.Size().Width(),
		Height:  w.Size().Height(),
		TypeID:  w.TypeID().UUID(),
		SceneID: w.SceneID().UUID(),
		Labels:  labels,
		TagIDs:  tagIDs,
		Version: w.Version().Number(),
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
