package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
)

// fakeRevisions is a RevisionRepository numbering the Revisions it is given
// and handing back the current one, its list and single ones.
type fakeRevisions struct {
	created        []uuid.UUID
	documents      []projectv0.ProjectFile
	times          []time.Time
	createErr      error
	current        runtimeapp.Revision
	hasCurrent     bool
	findErr        error
	summaries      []runtimeapp.RevisionSummary
	listErr        error
	byNumber       runtimeapp.Revision
	findByErr      error
	rolledBack     bool
	rollbackErr    error
	rollbackNumber int
}

func (r *fakeRevisions) CreateRevision(_ context.Context, projectID uuid.UUID, file projectv0.ProjectFile, now time.Time) (runtimeapp.Revision, error) {
	if r.createErr != nil {
		return runtimeapp.Revision{}, r.createErr
	}
	r.created = append(r.created, projectID)
	r.documents = append(r.documents, file)
	r.times = append(r.times, now)
	return runtimeapp.Revision{Number: len(r.created), DeployedAt: now, Cause: runtimeapp.RevisionCauseDeploy, Content: file}, nil
}

func (r *fakeRevisions) FindCurrent(context.Context, uuid.UUID) (runtimeapp.Revision, bool, error) {
	return r.current, r.hasCurrent, r.findErr
}

func (r *fakeRevisions) FindAll(context.Context, uuid.UUID) ([]runtimeapp.RevisionSummary, error) {
	return r.summaries, r.listErr
}

func (r *fakeRevisions) FindByNumber(context.Context, uuid.UUID, int) (runtimeapp.Revision, error) {
	return r.byNumber, r.findByErr
}

func (r *fakeRevisions) Rollback(_ context.Context, projectID uuid.UUID, n int, now time.Time) (runtimeapp.Revision, error) {
	if r.rollbackErr != nil {
		return runtimeapp.Revision{}, r.rollbackErr
	}
	r.rolledBack = true
	r.rollbackNumber = n
	rolledBackFrom := n
	return runtimeapp.Revision{
		Number:         len(r.created) + 1,
		DeployedAt:     now,
		Cause:          runtimeapp.RevisionCauseRollback,
		RolledBackFrom: &rolledBackFrom,
		Content:        projectv0.ProjectFile{Header: projectv0.Header{ProjectID: projectID}},
	}, nil
}

// recordingPublish keeps the events a service publishes.
func recordingPublish(events *[]event.Event) func(event.Event) {
	return func(e event.Event) { *events = append(*events, e) }
}

func TestRevisionService_Deploy_NumbersAndPublishesAfterTheRepositoryCommits(t *testing.T) {
	projectID := uuid.New()
	repo := &fakeRevisions{}
	var published []event.Event
	service := runtimeapp.NewRevisionService(repo, recordingPublish(&published))
	ctx := context.Background()

	first := runtimeapp.Revision{Content: projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID},
	}}
	one, err := service.Deploy(ctx, projectID, first.Content)
	if err != nil {
		t.Fatalf("Deploy 1: %v", err)
	}
	if one.Number != 1 {
		t.Errorf("first revision number %d, want 1", one.Number)
	}
	two, err := service.Deploy(ctx, projectID, first.Content)
	if err != nil {
		t.Fatalf("Deploy 2: %v", err)
	}
	if two.Number != 2 {
		t.Errorf("second revision number %d, want 2", two.Number)
	}
	if two.DeployedAt.Before(one.DeployedAt) {
		t.Errorf("deployed_at went back: %s then %s", one.DeployedAt, two.DeployedAt)
	}
	if len(repo.created) != 2 || repo.created[0] != projectID || repo.created[1] != projectID {
		t.Errorf("repository called for %v", repo.created)
	}
	if len(repo.times) != 2 || repo.times[0].IsZero() || repo.times[1].IsZero() {
		t.Errorf("deploy times %v", repo.times)
	}

	if len(published) != 2 {
		t.Fatalf("published %d events, want 2", len(published))
	}
	for i, e := range published {
		ev, ok := e.(runtimeapp.RevisionCreatedEvent)
		if !ok {
			t.Fatalf("event %d is a %T, not a RevisionCreatedEvent", i, e)
		}
		if ev.Type() != event.EventTypeRevisionCreated {
			t.Errorf("event %d is a %s", i, ev.Type())
		}
		if ev.ProjectID().UUID() != projectID {
			t.Errorf("event %d is of project %s, want %s", i, ev.ProjectID(), projectID)
		}
		if ev.Number() != i+1 {
			t.Errorf("event %d names revision %d, want %d", i, ev.Number(), i+1)
		}
	}
}

func TestRevisionService_Deploy_OfAForeignDocument_IsAnError(t *testing.T) {
	repo := &fakeRevisions{}
	var published []event.Event
	service := runtimeapp.NewRevisionService(repo, recordingPublish(&published))
	file := projectv0.ProjectFile{Header: projectv0.Header{ProjectID: uuid.New()}}

	if _, err := service.Deploy(context.Background(), uuid.New(), file); !errors.Is(err, runtimeapp.ErrInvalidInput) {
		t.Fatalf("a document of another project: %v, want ErrInvalidInput", err)
	}
	if len(repo.created) != 0 {
		t.Error("the repository was called for a foreign document")
	}
	if len(published) != 0 {
		t.Error("a foreign document published an event")
	}
}

func TestRevisionService_Deploy_OfAFailedCommit_PublishesNothing(t *testing.T) {
	repo := &fakeRevisions{createErr: errors.New("database is down")}
	var published []event.Event
	service := runtimeapp.NewRevisionService(repo, recordingPublish(&published))
	projectID := uuid.New()
	file := projectv0.ProjectFile{Header: projectv0.Header{ProjectID: projectID}}

	if _, err := service.Deploy(context.Background(), projectID, file); err == nil {
		t.Fatal("a failed deploy returned no error")
	}
	if len(published) != 0 {
		t.Errorf("a failed deploy published %d events", len(published))
	}
}

func TestRevisionService_FindCurrent(t *testing.T) {
	projectID := uuid.New()
	want := runtimeapp.Revision{Number: 4}
	repo := &fakeRevisions{current: want, hasCurrent: true}
	service := runtimeapp.NewRevisionService(repo, func(event.Event) {})

	found, ok, err := service.FindCurrent(context.Background(), projectID)
	if err != nil {
		t.Fatalf("FindCurrent: %v", err)
	}
	if !ok || found.Number != want.Number {
		t.Errorf("FindCurrent = %+v, %t, want %+v, true", found, ok, want)
	}

	repo.hasCurrent = false
	if _, ok, err := service.FindCurrent(context.Background(), projectID); err != nil || ok {
		t.Errorf("a project without a revision: %t, %v", ok, err)
	}

	repo.findErr = errors.New("database is down")
	if _, _, err := service.FindCurrent(context.Background(), projectID); err == nil {
		t.Error("a failed read returned no error")
	}
}

func TestRevisionService_ListAndFind(t *testing.T) {
	projectID := uuid.New()
	rolledBackFrom := 1
	repo := &fakeRevisions{
		summaries: []runtimeapp.RevisionSummary{
			{Number: 2, Cause: runtimeapp.RevisionCauseRollback, RolledBackFrom: &rolledBackFrom},
			{Number: 1, Cause: runtimeapp.RevisionCauseDeploy},
		},
		byNumber: runtimeapp.Revision{Number: 1, Cause: runtimeapp.RevisionCauseDeploy},
	}
	service := runtimeapp.NewRevisionService(repo, func(event.Event) {})
	ctx := context.Background()

	summaries, err := service.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 2 || summaries[0].Number != 2 || summaries[0].Cause != runtimeapp.RevisionCauseRollback {
		t.Errorf("List = %+v, want the newest first with its cause", summaries)
	}

	found, err := service.Find(ctx, projectID, 1)
	if err != nil || found.Number != 1 || found.Cause != runtimeapp.RevisionCauseDeploy {
		t.Errorf("Find = %+v, %v", found, err)
	}

	repo.listErr = errors.New("database is down")
	if _, err := service.List(ctx, projectID); err == nil {
		t.Error("a failed list returned no error")
	}
	repo.findByErr = runtimeapp.ErrRevisionNotFound
	if _, err := service.Find(ctx, projectID, 9); !errors.Is(err, runtimeapp.ErrRevisionNotFound) {
		t.Errorf("Find of a missing revision: %v, want ErrRevisionNotFound", err)
	}
}

func TestRevisionService_Rollback_PublishesAfterTheCommit(t *testing.T) {
	projectID := uuid.New()
	repo := &fakeRevisions{}
	var published []event.Event
	service := runtimeapp.NewRevisionService(repo, recordingPublish(&published))
	ctx := context.Background()

	revision, err := service.Rollback(ctx, projectID, 2)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if !repo.rolledBack || repo.rollbackNumber != 2 {
		t.Errorf("the repository was asked to roll back %d, called %v", repo.rollbackNumber, repo.rolledBack)
	}
	if revision.Cause != runtimeapp.RevisionCauseRollback || revision.RolledBackFrom == nil || *revision.RolledBackFrom != 2 {
		t.Errorf("the rolled-back revision: %+v", revision)
	}
	if len(published) != 1 {
		t.Fatalf("published %d events, want 1", len(published))
	}
	ev, ok := published[0].(runtimeapp.RevisionCreatedEvent)
	if !ok {
		t.Fatalf("published a %T, not a RevisionCreatedEvent", published[0])
	}
	if ev.Number() != revision.Number || ev.ProjectID().UUID() != projectID {
		t.Errorf("the event names revision %d of project %s", ev.Number(), ev.ProjectID())
	}
}

func TestRevisionService_Rollback_OfARefusedOne_PublishesNothing(t *testing.T) {
	for name, target := range map[string]error{
		"nothing to rollback": runtimeapp.ErrNothingToRollback,
		"missing revision":    runtimeapp.ErrRevisionNotFound,
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRevisions{rollbackErr: target}
			var published []event.Event
			service := runtimeapp.NewRevisionService(repo, recordingPublish(&published))

			if _, err := service.Rollback(context.Background(), uuid.New(), 1); !errors.Is(err, target) {
				t.Fatalf("Rollback: %v, want %v", err, target)
			}
			if len(published) != 0 {
				t.Errorf("a refused rollback published %d events", len(published))
			}
		})
	}
}
