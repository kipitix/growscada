package settings

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// Order is the order one person sees the Organization's libraries in: the
// ones they placed, first to last. It names only what they moved: a library it
// does not name follows them, by name, and an ID it names that no live library
// has anymore is ignored — the order is a view of the content, never an
// invariant about it, so a library deleted elsewhere never makes the order
// unwritable (task 81, ADR 0024).
//
// Value Object.
type Order struct {
	ids []uuid.UUID
}

var _ fmt.Stringer = Order{}

// NewOrder creates an Order from the IDs a client states. The zero ID is
// dropped and a repeated one keeps its first place, so one order has one
// representation however it was sent.
func NewOrder(ids []uuid.UUID) Order {
	unique := make([]uuid.UUID, 0, len(ids))
	for _, anID := range ids {
		if anID == uuid.Nil || slices.Contains(unique, anID) {
			continue
		}
		unique = append(unique, anID)
	}
	return Order{ids: unique}
}

// IDs returns the IDs of the order, first to last; never nil.
func (o Order) IDs() []uuid.UUID {
	if len(o.ids) == 0 {
		return []uuid.UUID{}
	}
	return slices.Clone(o.ids)
}

// String implements [fmt.Stringer].
func (o Order) String() string {
	ids := make([]string, len(o.ids))
	for i, anID := range o.ids {
		ids[i] = anID.String()
	}
	return "[" + strings.Join(ids, ", ") + "]"
}
