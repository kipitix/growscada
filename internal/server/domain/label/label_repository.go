package label

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// Repository stores the Labels of an Organization. It is not part of a Draft
// and states no Version: a label is metadata of the Organization, changed in
// one transaction of its own (task 85_2; ADR 0025).
type Repository interface {
	// NextID returns the ID of a label about to be created.
	NextID() id.ID[Label]
	// FindAll returns the Organization's labels, ordered by name ignoring
	// case.
	FindAll(ctx context.Context, organizationID id.ID[organization.Organization]) ([]Label, error)
	// FindByID returns the label; one the Organization lacks is
	// ErrLabelNotFound.
	FindByID(ctx context.Context, organizationID id.ID[organization.Organization], labelID id.ID[Label]) (Label, error)
	// Create stores a new label with the events it recorded. A name another
	// label of the Organization holds, ignoring case, is ErrLabelNameTaken.
	Create(ctx context.Context, l Label) (Label, error)
	// Update changes a stored label's name and color with the events it
	// recorded. A missing label is ErrLabelNotFound, a taken name
	// ErrLabelNameTaken.
	Update(ctx context.Context, l Label) (Label, error)
	// Delete removes the label and its assignments with the events it
	// recorded. A missing label is ErrLabelNotFound.
	Delete(ctx context.Context, l Label) error
}
