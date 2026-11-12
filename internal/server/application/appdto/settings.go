package appdto

import (
	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/settings"
)

// LibraryOrder is the application-layer DTO of a person's library order: the
// Version every write of it states, and the IDs of the libraries the person
// placed, first to last (task 81, ADR 0024). Version 0 with no ID is a person
// who never reordered.
type LibraryOrder struct {
	Version    int
	LibraryIDs []uuid.UUID
}

// NewLibraryOrder creates the DTO from the settings aggregate.
func NewLibraryOrder(s settings.Settings) LibraryOrder {
	return LibraryOrder{
		Version:    s.Version().Number(),
		LibraryIDs: s.LibraryOrder().IDs(),
	}
}
