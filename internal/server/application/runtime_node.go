package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// RuntimeNode is the Engineering node's port to the Runtime node (ADR 0011):
// the only way the Engineering node reaches the Tag states. The server
// implements it in-process today, over HTTP later (task 52). Every method
// takes the Project first.
//
// Until Deploy exists (task 35) the Runtime node learns the Tag definitions
// from the Draft through it: the Tag creation upserts its definition right
// after commit, and NewRuntimeTagDefinitionsPolicy brings the copy into line
// with the Draft after every tag_created and tag_deleted.
type RuntimeNode interface {
	// UpsertTagDefinitions gives the Runtime node the definitions of Tags: a
	// Tag without a state gets its initial one, a Tag with one keeps it.
	// Idempotent.
	UpsertTagDefinitions(ctx context.Context, projectID uuid.UUID, defs []appdto.TagDefinition) error
	// RemoveTagStates removes the states of the Tags; one without a state is
	// skipped.
	RemoveTagStates(ctx context.Context, projectID uuid.UUID, tagIDs []uuid.UUID) error
	// RemoveProjectTagStates removes the states of every Tag of the Project.
	RemoveProjectTagStates(ctx context.Context, projectID uuid.UUID) error
	// FindTagStates returns the states of the Tags with the given IDs; a Tag
	// without a state is left out.
	FindTagStates(ctx context.Context, projectID uuid.UUID, tagIDs []uuid.UUID) ([]appdto.TagState, error)
}
