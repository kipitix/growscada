package picker

import (
	"slices"

	"github.com/google/uuid"
)

// Order is the per-user order of the entities of a page and the Version its
// next write states (task 81, task 82): the IDs the person placed, first to
// last, and the Version the store holds them at, 0 for an order never written.
type Order struct {
	IDs     []uuid.UUID
	Version int
}

// reordered returns the IDs of shown with the entity at from moved to to, or
// nil when nothing moves. The arithmetic of a drop lives here, pure, so it is
// tested without a browser, and the browser's gesture only reports two
// positions (task 82).
func reordered(shown []Item, from, to int) []uuid.UUID {
	if from == to || from < 0 || to < 0 || from >= len(shown) || to >= len(shown) {
		return nil
	}
	moved := slices.Clone(shown)
	item := moved[from]
	moved = slices.Delete(moved, from, from+1)
	moved = slices.Insert(moved, to, item)
	ids := make([]uuid.UUID, len(moved))
	for i, it := range moved {
		ids[i] = it.ID
	}
	return ids
}
