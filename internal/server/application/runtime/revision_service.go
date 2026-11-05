package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/event"
)

// RevisionService is the Runtime node's service of Revisions: the Deploy that
// creates one, the reads of the current one and of the list and a single
// Revision, and the Rollback that makes an earlier one operational again
// (task 35, ADR 0014). The number and the time of a Revision are the Runtime
// node's to assign; the Engineering node reaches the service through its
// RuntimeNode port.
type RevisionService interface {
	// Deploy stores file as the new Revision of the Project and publishes
	// revision_created once the Revision is committed. The document must name
	// the Project it is deployed to.
	Deploy(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (Revision, error)
	// DeployFile stores file as the new Revision of the Project with cause
	// file (ADR 0018): the document is self-contained, so the Runtime node
	// reads no Draft and no Library. It publishes revision_created as a
	// Deploy does.
	DeployFile(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (Revision, error)
	// FindCurrent returns the latest Revision of the Project, and whether the
	// Project has one at all: a Project is not deployed until it does.
	FindCurrent(ctx context.Context, projectID uuid.UUID) (Revision, bool, error)
	// List returns the Project's Revisions, newest first, without their
	// documents. A Project without a Revision has an empty list.
	// project.ErrProjectNotFound if the Project is not live.
	List(ctx context.Context, projectID uuid.UUID) ([]RevisionSummary, error)
	// Find returns the Revision n of the Project with its document.
	// ErrRevisionNotFound if there is none, project.ErrProjectNotFound if the
	// Project is not live.
	Find(ctx context.Context, projectID uuid.UUID, n int) (Revision, error)
	// Rollback copies the Revision n into a new one and publishes
	// revision_created once it is committed. ErrRevisionNotFound if the
	// Project has no such Revision, ErrNothingToRollback if its document is
	// the running one and project.ErrProjectNotFound if the Project is not
	// live.
	Rollback(ctx context.Context, projectID uuid.UUID, n int) (Revision, error)
}

type revisionServiceImpl struct {
	revisions RevisionRepository
	// publish delivers revision_created after the commit, at-most-once, like
	// tag_updated (ADR 0011): the event is no aggregate change and leaves no
	// outbox row.
	publish func(event.Event)
	now     func() time.Time
}

var _ RevisionService = (*revisionServiceImpl)(nil)

// NewRevisionService returns the service of the Revisions; publish delivers
// revision_created (the EventBus of the SSE streams).
func NewRevisionService(someRevisions RevisionRepository, publish func(event.Event)) RevisionService {
	return &revisionServiceImpl{revisions: someRevisions, publish: publish, now: time.Now}
}

func (s revisionServiceImpl) Deploy(ctx context.Context, rawProjectID uuid.UUID, file projectv0.ProjectFile) (Revision, error) {
	if file.ProjectID != rawProjectID {
		return Revision{}, invalidInput(fmt.Errorf("cannot deploy a document of project %s to project %s", file.ProjectID, rawProjectID))
	}
	revision, err := s.revisions.CreateRevision(ctx, rawProjectID, file, RevisionCauseDeploy, s.now())
	if err != nil {
		return Revision{}, fmt.Errorf("cannot deploy project %s: %w", rawProjectID, err)
	}
	s.publish(NewRevisionCreatedEvent(revision))
	return revision, nil
}

func (s revisionServiceImpl) DeployFile(ctx context.Context, rawProjectID uuid.UUID, file projectv0.ProjectFile) (Revision, error) {
	if file.ProjectID != rawProjectID {
		return Revision{}, invalidInput(fmt.Errorf("cannot deploy a document of project %s to project %s", file.ProjectID, rawProjectID))
	}
	revision, err := s.revisions.CreateRevision(ctx, rawProjectID, file, RevisionCauseFile, s.now())
	if err != nil {
		return Revision{}, fmt.Errorf("cannot deploy a project file to project %s: %w", rawProjectID, err)
	}
	s.publish(NewRevisionCreatedEvent(revision))
	return revision, nil
}

func (s revisionServiceImpl) FindCurrent(ctx context.Context, rawProjectID uuid.UUID) (Revision, bool, error) {
	found, ok, err := s.revisions.FindCurrent(ctx, rawProjectID)
	if err != nil {
		return Revision{}, false, fmt.Errorf("cannot read the current revision of project %s: %w", rawProjectID, err)
	}
	return found, ok, nil
}

func (s revisionServiceImpl) List(ctx context.Context, rawProjectID uuid.UUID) ([]RevisionSummary, error) {
	summaries, err := s.revisions.FindAll(ctx, rawProjectID)
	if err != nil {
		return nil, fmt.Errorf("cannot read the revisions of project %s: %w", rawProjectID, err)
	}
	return summaries, nil
}

func (s revisionServiceImpl) Find(ctx context.Context, rawProjectID uuid.UUID, n int) (Revision, error) {
	found, err := s.revisions.FindByNumber(ctx, rawProjectID, n)
	if err != nil {
		return Revision{}, fmt.Errorf("cannot read revision %d of project %s: %w", n, rawProjectID, err)
	}
	return found, nil
}

func (s revisionServiceImpl) Rollback(ctx context.Context, rawProjectID uuid.UUID, n int) (Revision, error) {
	revision, err := s.revisions.Rollback(ctx, rawProjectID, n, s.now())
	if err != nil {
		return Revision{}, fmt.Errorf("cannot roll project %s back to revision %d: %w", rawProjectID, n, err)
	}
	s.publish(NewRevisionCreatedEvent(revision))
	return revision, nil
}
