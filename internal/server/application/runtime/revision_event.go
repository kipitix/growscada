package runtime

import (
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// RevisionCreatedEvent is published by the Runtime node once a Deploy has
// committed its Revision (task 35, Q11): never stored in the outbox, delivered
// at-most-once, like tag_updated (ADR 0011). It is a project.ContentEvent, so
// the event stream routes it to the readers of the Project's configuration;
// Number and DeployedAt let them update what they show without reading the
// Revision back.
type RevisionCreatedEvent interface {
	project.ContentEvent
	Number() int
	DeployedAt() time.Time
}

type revisionCreatedEventImpl struct {
	event.Base
	project.Content
	number     int
	deployedAt time.Time
}

var _ RevisionCreatedEvent = (*revisionCreatedEventImpl)(nil)

// NewRevisionCreatedEvent returns the event announcing aRevision as the
// Project's running configuration.
func NewRevisionCreatedEvent(aRevision Revision, opts ...event.EventOption) RevisionCreatedEvent {
	projectID := id.NewID(id.IDWithUUID[project.Project](aRevision.Content.ProjectID))
	return &revisionCreatedEventImpl{
		Base:       event.NewBase(event.EventTypeRevisionCreated, opts...),
		Content:    project.NewContent(projectID),
		number:     aRevision.Number,
		deployedAt: aRevision.DeployedAt,
	}
}

func (e revisionCreatedEventImpl) Number() int           { return e.number }
func (e revisionCreatedEventImpl) DeployedAt() time.Time { return e.deployedAt }
