package application

import (
	"context"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// RuntimeNode is the Engineering node's port to the Runtime node (ADR 0011):
// the only way the Engineering node reaches the Tag states, the running
// Revision and the Device Tokens. The server implements it in-process today,
// over HTTP later (task 52). Every method takes the Project first.
//
// A Deploy hands the Runtime node the whole ProjectFile, which it stores as a
// numbered Revision and from which it rebuilds its copies of the Tag
// definitions, the Devices and the Environments. The copies are the running
// Revision's alone: nothing of the Draft reaches this port (task 35, part 4).
type RuntimeNode interface {
	// RemoveProjectTagStates removes the states of every Tag of the Project.
	RemoveProjectTagStates(ctx context.Context, projectID uuid.UUID) error
	// RemoveProjectDeviceTokens removes every Device Token of the Project,
	// and the ones already resolved in memory with it: the Project's tokens
	// go when its row goes, and a token of a removed Project must not
	// authenticate.
	RemoveProjectDeviceTokens(ctx context.Context, projectID uuid.UUID) error
	// FindTagStates returns the states of the Tags with the given IDs; a Tag
	// without a state is left out.
	FindTagStates(ctx context.Context, projectID uuid.UUID, tagIDs []uuid.UUID) ([]appdto.TagState, error)
	// Deploy stores file as the new Revision of the Project and returns it:
	// the number and the time are the Runtime node's to assign. The document
	// must name the Project it is deployed to.
	Deploy(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (appdto.Revision, error)
	// DeployFile stores file as the new Revision of the Project with cause
	// file (ADR 0018): the document needs no Draft and no Library, and the
	// Revision is published as any Deploy's is. The document must name the
	// Project it is deployed to.
	DeployFile(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (appdto.Revision, error)
	// FindCurrentRevision returns the latest Revision of the Project, and
	// whether it has one at all: a Project is not deployed until it does.
	FindCurrentRevision(ctx context.Context, projectID uuid.UUID) (appdto.Revision, bool, error)
	// FindRevision returns the Revision n of the Project with its document:
	// the Runtime node's stored ProjectFile, as GET …/revisions/{n} serves it.
	FindRevision(ctx context.Context, projectID uuid.UUID, n int) (appdto.Revision, error)
}
