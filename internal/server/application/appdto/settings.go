package appdto

import (
	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/settings"
)

// Order is the application-layer DTO of a person's order of the Libraries or
// the Projects: the Version every write of it states, and the IDs the person
// placed, first to last (task 81, task 83, ADR 0024). Version 0 with no ID is
// a person who never reordered.
type Order struct {
	Version int
	IDs     []uuid.UUID
}

// NewOrder creates the DTO from the settings aggregate.
func NewOrder(s settings.Settings) Order {
	return Order{Version: s.Version().Number(), IDs: s.Order().IDs()}
}
