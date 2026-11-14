package label

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// Repository stores the Labels of an Organization. It is not part of a Draft
// and states no Version: a label is metadata of the Organization, changed in
// one transaction of its own (task 85_2; ADR 0025). Its assignments — its
// entities — are written by the same repository, each in the label's own
// transaction (task 85_3).
type Repository interface {
	// NextID returns the ID of a label about to be created.
	NextID() id.ID[Label]
	// FindAll returns the Organization's labels, ordered by name ignoring
	// case, with the saved states each marks.
	FindAll(ctx context.Context, organizationID id.ID[organization.Organization]) ([]Label, error)
	// FindByID returns the label with its assignments; one the Organization
	// lacks is ErrLabelNotFound.
	FindByID(ctx context.Context, organizationID id.ID[organization.Organization], labelID id.ID[Label]) (Label, error)
	// Create stores a new label with the events it recorded. A name another
	// label of the Organization holds, ignoring case, is ErrLabelNameTaken.
	Create(ctx context.Context, l Label) (Label, error)
	// Update changes a stored label's name and color with the events it
	// recorded; the assignments are untouched. A missing label is
	// ErrLabelNotFound, a taken name ErrLabelNameTaken.
	Update(ctx context.Context, l Label) (Label, error)
	// Delete removes the label and its assignments with the events it
	// recorded. A missing label is ErrLabelNotFound.
	Delete(ctx context.Context, l Label) error

	// Assign stores the mark the label recorded on the Commit commitID and its
	// events, in one transaction that locks the label's row and keeps the
	// Commit from being removed while it runs: a label deleted concurrently is
	// ErrLabelNotFound, a Commit this server lacks draft.ErrCommitNotFound. An
	// assignment already stored is not an error and writes no event (its key
	// makes it one row).
	Assign(ctx context.Context, l Label, commitID id.ID[draft.Commit]) (Label, error)
	// Unassign removes the mark of the Commit commitID and writes the events
	// the label recorded; a mark that is not there is not an error and writes
	// none.
	Unassign(ctx context.Context, l Label, commitID id.ID[draft.Commit]) (Label, error)
	// LabelsOfScope returns the labels of every saved state of the Draft of
	// scope that carries one, keyed by the Commit's number and ordered by name
	// ignoring case: one query for the history of a Draft (task 85_3).
	LabelsOfScope(ctx context.Context, organizationID id.ID[organization.Organization], scope draft.Scope) (map[int][]Label, error)
}
