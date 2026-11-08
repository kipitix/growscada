package runtime

import (
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// DeploymentCreatedEvent is published by the Runtime node once a Deploy has
// committed its Deployment (task 35, Q11): never stored in the outbox, delivered
// at-most-once, like tag_updated (ADR 0011). It is a project.ContentEvent, so
// the event stream routes it to the readers of the Project's configuration;
// Number and DeployedAt let them update what they show without reading the
// Deployment back.
type DeploymentCreatedEvent interface {
	project.ContentEvent
	Number() int
	DeployedAt() time.Time
	// ReleaseNumber is the Project Release the Deployment made (task 72_4);
	// nil for a Deployment from a file.
	ReleaseNumber() *int
}

type deploymentCreatedEventImpl struct {
	event.Base
	project.Content
	number        int
	deployedAt    time.Time
	releaseNumber *int
}

var _ DeploymentCreatedEvent = (*deploymentCreatedEventImpl)(nil)

// NewDeploymentCreatedEvent returns the event announcing aDeployment as the
// Project's running configuration.
func NewDeploymentCreatedEvent(aDeployment Deployment, opts ...event.EventOption) DeploymentCreatedEvent {
	projectID := id.NewID(id.IDWithUUID[project.Project](aDeployment.Content.ProjectID))
	return &deploymentCreatedEventImpl{
		Base:          event.NewBase(event.EventTypeDeploymentCreated, opts...),
		Content:       project.NewContent(projectID),
		number:        aDeployment.Number,
		deployedAt:    aDeployment.DeployedAt,
		releaseNumber: aDeployment.ReleaseNumber,
	}
}

func (e deploymentCreatedEventImpl) Number() int           { return e.number }
func (e deploymentCreatedEventImpl) DeployedAt() time.Time { return e.deployedAt }
func (e deploymentCreatedEventImpl) ReleaseNumber() *int   { return e.releaseNumber }
