package tagstate

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// Repository stores the Tag states of the Runtime node, with their copies of
// the Tag definitions (ADR 0011). A Tag has a state once its definition was
// upserted, until it is deleted. Every method names the Project: a Tag of
// another Project is not found.
type Repository interface {
	// FindInLiveProject returns the state of the Tag of the Project, checking
	// in the same query that the Project is live and of the Organization: the
	// hot path of Tag values.
	// Returns tag.ErrTagNotFound if there is none or the Project is not one of
	// those, or an error on failure.
	FindInLiveProject(context.Context, id.ID[project.Project], id.ID[tag.Tag], id.ID[organization.Organization]) (TagState, error)

	// FindByIDs returns the states of the Tags of the Project with the given
	// IDs, read in one query; a Tag without a state is left out.
	FindByIDs(context.Context, id.ID[project.Project], []id.ID[tag.Tag]) (map[id.ID[tag.Tag]]TagState, error)

	// IsProjectLive reports whether the Project is live and of the
	// Organization: to tell a missing Project from a missing Tag.
	IsProjectLive(context.Context, id.ID[project.Project], id.ID[organization.Organization]) (bool, error)

	// Write stores aState as its Tag's state whatever is stored (the latest
	// write wins) and returns it with the Seq and WrittenAt of the write.
	// The change is recorded in the Journal in the same transaction, and only
	// when the value or the Quality differs from the stored one: a repeat
	// writes no entry, keeps the Seq and moves WrittenAt alone (ADR 0019). The
	// Tag's row is locked for the comparison, so two writers of one Tag are
	// serialized and take their positions in the lock's order. It writes only
	// for aSupplier, the writer's Supplier: one that is not the Tag's supplier
	// is refused in the same query as the Project and TagType checks (ADR
	// 0011), the hot path paying no extra read.
	// Returns tag.ErrTagNotFound if the Tag has no state (its definition was
	// removed), tag.ErrTagNotSuppliedByDevice if aSupplier is not the Tag's
	// supplier (or names no Device while one supplies it: a Tag a Device
	// supplies is written by that Device alone), or an error on failure. The
	// bool reports whether the state changed.
	Write(context.Context, TagState, tag.Supplier) (TagState, bool, error)

	// UpsertDefinitions stores the definitions of the Tags of the Project: a
	// Tag without a state gets its Initial one, a Tag with one keeps it.
	// Idempotent.
	UpsertDefinitions(context.Context, id.ID[project.Project], []Definition) error

	// Delete removes the states of the Tags of the Project with the given
	// IDs; one without a state is skipped.
	Delete(context.Context, id.ID[project.Project], []id.ID[tag.Tag]) error

	// DeleteAllOfProject removes the states of every Tag of the Project.
	DeleteAllOfProject(context.Context, id.ID[project.Project]) error
}
