package application

import (
	"context"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// RuntimeNode is the Engineering node's port to the Runtime node (ADR 0011):
// the only way the Engineering node reaches the Tag states, the running
// Deployment and the Device Tokens. The server implements it in-process today,
// over HTTP later (task 52). Every method takes the Project first.
//
// A Deploy hands the Runtime node the whole ProjectFile, which it stores as a
// numbered Deployment and from which it rebuilds its copies of the Tag
// definitions, the Devices and the Environments. The copies are the running
// Deployment's alone: nothing of the Draft reaches this port (task 35, part 4).
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
	// DeployRelease stores file as the new Deployment of the Project, made
	// from Release releaseNumber (task 72_4), and returns it: the number and
	// the time are the Runtime node's to assign. The document must name the
	// Project it is deployed to, and the Release the Project already runs is
	// ErrNothingToDeploy.
	DeployRelease(ctx context.Context, projectID uuid.UUID, releaseNumber int, file projectv0.ProjectFile) (appdto.Deployment, error)
	// DeployFile stores file as the new Deployment of the Project with cause
	// file (ADR 0018): the document needs no Draft and no Library, and the
	// Deployment is published as any Deploy's is. The document must name the
	// Project it is deployed to.
	DeployFile(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (appdto.Deployment, error)
	// FindCurrentDeployment returns the latest Deployment of the Project, and
	// whether it has one at all: a Project is not deployed until it does.
	FindCurrentDeployment(ctx context.Context, projectID uuid.UUID) (appdto.Deployment, bool, error)
	// FindDeployment returns the Deployment n of the Project with its document:
	// the Runtime node's stored ProjectFile, as GET …/deployments/{n} serves it.
	FindDeployment(ctx context.Context, projectID uuid.UUID, n int) (appdto.Deployment, error)
	// DeploymentNamesLibrary reports whether the running Deployment of any live
	// Project names a Release of this Library in its ProjectFile (task 71): a
	// Library so named cannot be removed, because a Discard of that Deployment
	// could not restore the pin. The answer reads no Engineering schema.
	DeploymentNamesLibrary(ctx context.Context, libraryID uuid.UUID) (bool, error)
}
