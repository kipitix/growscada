package appdto

import (
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/label"
)

// Label is the application-layer DTO of a Label: its identity, name, optional
// color, when it was created and how many saved states it marks (task 85_2;
// ADR 0025). An empty Color is the theme's neutral color. AssignedCount is 0
// until task 85_3 adds the assignments.
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
		ID:        l.ID().UUID(),
		Name:      l.Name().String(),
		Color:     l.Color().String(),
		CreatedAt: l.CreatedAt(),
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
