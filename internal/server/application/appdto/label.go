package appdto

import (
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/label"
)

// Label is the application-layer DTO of a Label: its identity, name, optional
// color, when it was created and how many saved states it marks (task 85_2,
// task 85_3; ADR 0025). An empty Color is the theme's neutral color.
type Label struct {
	ID            uuid.UUID
	Name          string
	Color         string
	CreatedAt     time.Time
	AssignedCount int
}

// NewLabel creates a Label DTO from the domain aggregate.
func NewLabel(l label.Label) Label {
	return Label{
		ID:            l.ID().UUID(),
		Name:          l.Name().String(),
		Color:         l.Color().String(),
		CreatedAt:     l.CreatedAt(),
		AssignedCount: len(l.Assignments()),
	}
}

// NewLabels creates the DTOs of the labels, in their order.
func NewLabels(labels []label.Label) []Label {
	out := make([]Label, len(labels))
	for i, l := range labels {
		out[i] = NewLabel(l)
	}
	return out
}

// NewLabelMap creates the DTOs of the labels of each saved state, keyed by the
// saved state's number, in the order the map holds them.
func NewLabelMap(byNumber map[int][]label.Label) map[int][]Label {
	out := make(map[int][]Label, len(byNumber))
	for number, labels := range byNumber {
		out[number] = NewLabels(labels)
	}
	return out
}
