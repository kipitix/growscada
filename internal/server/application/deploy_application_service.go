package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// DeployService is the Engineering node's service of a Deploy (task 85_4): it
// makes a saved state — a Commit — of a Project its running Deployment. A
// A Release takes no part in it, and neither does a Draft Version: a Commit is
// immutable and a Deploy changes no Draft (ADR 0021). The document is the
// Commit's ProjectFile; the Runtime node stores it and assigns the number and
// the time.
type DeployService interface {
	// Deploy makes Commit commitNumber of the live Project projectID its
	// running Deployment. Returns draft.ErrCommitNotFound for a number the
	// Project does not have, project.ErrProjectNotFound if the Project is not
	// live, ErrNothingToDeploy if that saved state is the one it runs already,
	// and invalid input for a number below 1.
	Deploy(ctx context.Context, projectID uuid.UUID, commitNumber int) (appdto.Deployment, error)
}

type deployServiceImpl struct {
	content ProjectContent
	runtime RuntimeNode
}

var _ DeployService = (*deployServiceImpl)(nil)

// NewDeployService returns the service of the Deploys of the live Projects
// aContent serves, made running through aRuntime.
func NewDeployService(aContent ProjectContent, aRuntime RuntimeNode) DeployService {
	return &deployServiceImpl{content: aContent, runtime: aRuntime}
}

func (s deployServiceImpl) Deploy(ctx context.Context, rawProjectID uuid.UUID, rawCommit int) (appdto.Deployment, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Deployment{}, err
	}
	number, err := commitNumber(rawCommit)
	if err != nil {
		return appdto.Deployment{}, err
	}
	commit, err := s.content.Drafts().FindCommit(ctx, s.content.ScopeOf(projectID), number)
	if err != nil {
		return appdto.Deployment{}, err
	}
	file, ok := commit.Content().ProjectFile()
	if !ok {
		return appdto.Deployment{}, fmt.Errorf("saved state %d of project %s holds no project file", number, projectID)
	}
	deployed, err := s.runtime.DeployCommit(ctx, projectID.UUID(), number, file)
	if err != nil {
		if errors.Is(err, ErrNothingToDeploy) {
			return appdto.Deployment{}, fmt.Errorf("%w: saved state %d of project %s runs already", ErrNothingToDeploy, number, projectID)
		}
		return appdto.Deployment{}, fmt.Errorf("cannot deploy saved state %d of project %s: %w", number, projectID, err)
	}
	return deployed, nil
}
