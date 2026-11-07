package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/event"
)

// DeploymentService is the Runtime node's service of Deployments: the Deploy that
// creates one, the reads of the current one and of the list and a single
// Deployment, and the Rollback that makes an earlier one operational again
// (task 35, ADR 0014). The number and the time of a Deployment are the Runtime
// node's to assign; the Engineering node reaches the service through its
// RuntimeNode port.
type DeploymentService interface {
	// Deploy stores file as the new Deployment of the Project and publishes
	// deployment_created once the Deployment is committed. The document must name
	// the Project it is deployed to.
	Deploy(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (Deployment, error)
	// DeployFile stores file as the new Deployment of the Project with cause
	// file (ADR 0018): the document is self-contained, so the Runtime node
	// reads no Draft and no Library. It publishes deployment_created as a
	// Deploy does.
	DeployFile(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (Deployment, error)
	// FindCurrent returns the latest Deployment of the Project, and whether the
	// Project has one at all: a Project is not deployed until it does.
	FindCurrent(ctx context.Context, projectID uuid.UUID) (Deployment, bool, error)
	// List returns the Project's Deployments, newest first, without their
	// documents. A Project without a Deployment has an empty list.
	// project.ErrProjectNotFound if the Project is not live.
	List(ctx context.Context, projectID uuid.UUID) ([]DeploymentSummary, error)
	// Find returns the Deployment n of the Project with its document.
	// ErrDeploymentNotFound if there is none, project.ErrProjectNotFound if the
	// Project is not live.
	Find(ctx context.Context, projectID uuid.UUID, n int) (Deployment, error)
	// Rollback copies the Deployment n into a new one and publishes
	// deployment_created once it is committed. ErrDeploymentNotFound if the
	// Project has no such Deployment, ErrNothingToRollback if its document is
	// the running one and project.ErrProjectNotFound if the Project is not
	// live.
	Rollback(ctx context.Context, projectID uuid.UUID, n int) (Deployment, error)
}

type deploymentServiceImpl struct {
	deployments DeploymentRepository
	// publish delivers deployment_created after the commit, at-most-once, like
	// tag_updated (ADR 0011): the event is no aggregate change and leaves no
	// outbox row.
	publish func(event.Event)
	now     func() time.Time
}

var _ DeploymentService = (*deploymentServiceImpl)(nil)

// NewDeploymentService returns the service of the Deployments; publish delivers
// deployment_created (the EventBus of the SSE streams).
func NewDeploymentService(someDeployments DeploymentRepository, publish func(event.Event)) DeploymentService {
	return &deploymentServiceImpl{deployments: someDeployments, publish: publish, now: time.Now}
}

func (s deploymentServiceImpl) Deploy(ctx context.Context, rawProjectID uuid.UUID, file projectv0.ProjectFile) (Deployment, error) {
	if file.ProjectID != rawProjectID {
		return Deployment{}, invalidInput(fmt.Errorf("cannot deploy a document of project %s to project %s", file.ProjectID, rawProjectID))
	}
	deployment, err := s.deployments.CreateDeployment(ctx, rawProjectID, file, DeploymentCauseDeploy, s.now())
	if err != nil {
		return Deployment{}, fmt.Errorf("cannot deploy project %s: %w", rawProjectID, err)
	}
	s.publish(NewDeploymentCreatedEvent(deployment))
	return deployment, nil
}

func (s deploymentServiceImpl) DeployFile(ctx context.Context, rawProjectID uuid.UUID, file projectv0.ProjectFile) (Deployment, error) {
	if file.ProjectID != rawProjectID {
		return Deployment{}, invalidInput(fmt.Errorf("cannot deploy a document of project %s to project %s", file.ProjectID, rawProjectID))
	}
	deployment, err := s.deployments.CreateDeployment(ctx, rawProjectID, file, DeploymentCauseFile, s.now())
	if err != nil {
		return Deployment{}, fmt.Errorf("cannot deploy a project file to project %s: %w", rawProjectID, err)
	}
	s.publish(NewDeploymentCreatedEvent(deployment))
	return deployment, nil
}

func (s deploymentServiceImpl) FindCurrent(ctx context.Context, rawProjectID uuid.UUID) (Deployment, bool, error) {
	found, ok, err := s.deployments.FindCurrent(ctx, rawProjectID)
	if err != nil {
		return Deployment{}, false, fmt.Errorf("cannot read the current deployment of project %s: %w", rawProjectID, err)
	}
	return found, ok, nil
}

func (s deploymentServiceImpl) List(ctx context.Context, rawProjectID uuid.UUID) ([]DeploymentSummary, error) {
	summaries, err := s.deployments.FindAll(ctx, rawProjectID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the deployments of project %s: %w", rawProjectID, err)
	}
	return summaries, nil
}

func (s deploymentServiceImpl) Find(ctx context.Context, rawProjectID uuid.UUID, n int) (Deployment, error) {
	found, err := s.deployments.FindByNumber(ctx, rawProjectID, n)
	if err != nil {
		return Deployment{}, fmt.Errorf("cannot read deployment %d of project %s: %w", n, rawProjectID, err)
	}
	return found, nil
}

func (s deploymentServiceImpl) Rollback(ctx context.Context, rawProjectID uuid.UUID, n int) (Deployment, error) {
	deployment, err := s.deployments.Rollback(ctx, rawProjectID, n, s.now())
	if err != nil {
		return Deployment{}, fmt.Errorf("cannot roll project %s back to deployment %d: %w", rawProjectID, n, err)
	}
	s.publish(NewDeploymentCreatedEvent(deployment))
	return deployment, nil
}
