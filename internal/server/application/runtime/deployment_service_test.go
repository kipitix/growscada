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

// fakeDeployments is a DeploymentRepository numbering the Deployments it is given
// and handing back the current one, its list and single ones.
type fakeDeployments struct {
	created        []uuid.UUID
	documents      []projectv0.ProjectFile
	causes         []runtimeapp.DeploymentCause
	times          []time.Time
	createErr      error
	current        runtimeapp.Deployment
	hasCurrent     bool
	findErr        error
	summaries      []runtimeapp.DeploymentSummary
	listErr        error
	byNumber       runtimeapp.Deployment
	findByErr      error
	rolledBack     bool
	rollbackErr    error
	rollbackNumber int
	namedLibrary   bool
	namedErr       error
}

func (r *fakeDeployments) CreateDeployment(_ context.Context, projectID uuid.UUID, file projectv0.ProjectFile, cause runtimeapp.DeploymentCause, now time.Time) (runtimeapp.Deployment, error) {
	if r.createErr != nil {
		return runtimeapp.Deployment{}, r.createErr
	}
	r.created = append(r.created, projectID)
	r.documents = append(r.documents, file)
	r.causes = append(r.causes, cause)
	r.times = append(r.times, now)
	return runtimeapp.Deployment{Number: len(r.created), DeployedAt: now, Cause: cause, Content: file}, nil
}

func (r *fakeDeployments) FindCurrent(context.Context, uuid.UUID) (runtimeapp.Deployment, bool, error) {
	return r.current, r.hasCurrent, r.findErr
}

func (r *fakeDeployments) FindAll(context.Context, uuid.UUID) ([]runtimeapp.DeploymentSummary, error) {
	return r.summaries, r.listErr
}

func (r *fakeDeployments) FindByNumber(context.Context, uuid.UUID, int) (runtimeapp.Deployment, error) {
	return r.byNumber, r.findByErr
}

func (r *fakeDeployments) DeploymentNamesLibrary(context.Context, uuid.UUID) (bool, error) {
	return r.namedLibrary, r.namedErr
}

func (r *fakeDeployments) Rollback(_ context.Context, projectID uuid.UUID, n int, now time.Time) (runtimeapp.Deployment, error) {
	if r.rollbackErr != nil {
		return runtimeapp.Deployment{}, r.rollbackErr
	}
	r.rolledBack = true
	r.rollbackNumber = n
	rolledBackFrom := n
	return runtimeapp.Deployment{
		Number:         len(r.created) + 1,
		DeployedAt:     now,
		Cause:          runtimeapp.DeploymentCauseRollback,
		RolledBackFrom: &rolledBackFrom,
		Content:        projectv0.ProjectFile{Header: projectv0.Header{ProjectID: projectID}},
	}, nil
}

// recordingPublish keeps the events a service publishes.
func recordingPublish(events *[]event.Event) func(event.Event) {
	return func(e event.Event) { *events = append(*events, e) }
}

func TestDeploymentService_Deploy_NumbersAndPublishesAfterTheRepositoryCommits(t *testing.T) {
	projectID := uuid.New()
	repo := &fakeDeployments{}
	var published []event.Event
	service := runtimeapp.NewDeploymentService(repo, recordingPublish(&published))
	ctx := context.Background()

	first := runtimeapp.Deployment{Content: projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID},
	}}
	one, err := service.Deploy(ctx, projectID, first.Content)
	if err != nil {
		t.Fatalf("Deploy 1: %v", err)
	}
	if one.Number != 1 {
		t.Errorf("first deployment number %d, want 1", one.Number)
	}
	two, err := service.Deploy(ctx, projectID, first.Content)
	if err != nil {
		t.Fatalf("Deploy 2: %v", err)
	}
	if two.Number != 2 {
		t.Errorf("second deployment number %d, want 2", two.Number)
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
		ev, ok := e.(runtimeapp.DeploymentCreatedEvent)
		if !ok {
			t.Fatalf("event %d is a %T, not a DeploymentCreatedEvent", i, e)
		}
		if ev.Type() != event.EventTypeDeploymentCreated {
			t.Errorf("event %d is a %s", i, ev.Type())
		}
		if ev.ProjectID().UUID() != projectID {
			t.Errorf("event %d is of project %s, want %s", i, ev.ProjectID(), projectID)
		}
		if ev.Number() != i+1 {
			t.Errorf("event %d names deployment %d, want %d", i, ev.Number(), i+1)
		}
	}
}

// TestDeploymentService_DeployFile_StoresTheCauseFileAndPublishes: a file
// deploy is a Deployment with cause file, and every reader is told of it
// (ADR 0018).
func TestDeploymentService_DeployFile_StoresTheCauseFileAndPublishes(t *testing.T) {
	projectID := uuid.New()
	repo := &fakeDeployments{}
	var published []event.Event
	service := runtimeapp.NewDeploymentService(repo, recordingPublish(&published))
	file := projectv0.ProjectFile{Header: projectv0.Header{
		SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID,
	}}

	deployment, err := service.DeployFile(context.Background(), projectID, file)
	if err != nil {
		t.Fatalf("DeployFile: %v", err)
	}
	if deployment.Cause != runtimeapp.DeploymentCauseFile {
		t.Errorf("cause %q, want file", deployment.Cause)
	}
	if len(repo.causes) != 1 || repo.causes[0] != runtimeapp.DeploymentCauseFile {
		t.Errorf("repository causes %v, want one file", repo.causes)
	}
	if len(published) != 1 {
		t.Fatalf("published %d events, want 1", len(published))
	}
	if _, ok := published[0].(runtimeapp.DeploymentCreatedEvent); !ok {
		t.Errorf("event is a %T, not a DeploymentCreatedEvent", published[0])
	}
}

func TestDeploymentService_DeployFile_OfAForeignDocument_IsAnError(t *testing.T) {
	repo := &fakeDeployments{}
	service := runtimeapp.NewDeploymentService(repo, func(event.Event) {})
	file := projectv0.ProjectFile{Header: projectv0.Header{ProjectID: uuid.New()}}
	if _, err := service.DeployFile(context.Background(), uuid.New(), file); !errors.Is(err, runtimeapp.ErrInvalidInput) {
		t.Fatalf("a document of another project: %v, want ErrInvalidInput", err)
	}
	if len(repo.created) != 0 {
		t.Error("the repository was called for a foreign document")
	}
}

func TestDeploymentService_Deploy_OfAForeignDocument_IsAnError(t *testing.T) {
	repo := &fakeDeployments{}
	var published []event.Event
	service := runtimeapp.NewDeploymentService(repo, recordingPublish(&published))
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

func TestDeploymentService_Deploy_OfAFailedCommit_PublishesNothing(t *testing.T) {
	repo := &fakeDeployments{createErr: errors.New("database is down")}
	var published []event.Event
	service := runtimeapp.NewDeploymentService(repo, recordingPublish(&published))
	projectID := uuid.New()
	file := projectv0.ProjectFile{Header: projectv0.Header{ProjectID: projectID}}

	if _, err := service.Deploy(context.Background(), projectID, file); err == nil {
		t.Fatal("a failed deploy returned no error")
	}
	if len(published) != 0 {
		t.Errorf("a failed deploy published %d events", len(published))
	}
}

func TestDeploymentService_FindCurrent(t *testing.T) {
	projectID := uuid.New()
	want := runtimeapp.Deployment{Number: 4}
	repo := &fakeDeployments{current: want, hasCurrent: true}
	service := runtimeapp.NewDeploymentService(repo, func(event.Event) {})

	found, ok, err := service.FindCurrent(context.Background(), projectID)
	if err != nil {
		t.Fatalf("FindCurrent: %v", err)
	}
	if !ok || found.Number != want.Number {
		t.Errorf("FindCurrent = %+v, %t, want %+v, true", found, ok, want)
	}

	repo.hasCurrent = false
	if _, ok, err := service.FindCurrent(context.Background(), projectID); err != nil || ok {
		t.Errorf("a project without a deployment: %t, %v", ok, err)
	}

	repo.findErr = errors.New("database is down")
	if _, _, err := service.FindCurrent(context.Background(), projectID); err == nil {
		t.Error("a failed read returned no error")
	}
}

func TestDeploymentService_ListAndFind(t *testing.T) {
	projectID := uuid.New()
	rolledBackFrom := 1
	repo := &fakeDeployments{
		summaries: []runtimeapp.DeploymentSummary{
			{Number: 2, Cause: runtimeapp.DeploymentCauseRollback, RolledBackFrom: &rolledBackFrom},
			{Number: 1, Cause: runtimeapp.DeploymentCauseDeploy},
		},
		byNumber: runtimeapp.Deployment{Number: 1, Cause: runtimeapp.DeploymentCauseDeploy},
	}
	service := runtimeapp.NewDeploymentService(repo, func(event.Event) {})
	ctx := context.Background()

	summaries, err := service.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 2 || summaries[0].Number != 2 || summaries[0].Cause != runtimeapp.DeploymentCauseRollback {
		t.Errorf("List = %+v, want the newest first with its cause", summaries)
	}

	found, err := service.Find(ctx, projectID, 1)
	if err != nil || found.Number != 1 || found.Cause != runtimeapp.DeploymentCauseDeploy {
		t.Errorf("Find = %+v, %v", found, err)
	}

	repo.listErr = errors.New("database is down")
	if _, err := service.List(ctx, projectID); err == nil {
		t.Error("a failed list returned no error")
	}
	repo.findByErr = runtimeapp.ErrDeploymentNotFound
	if _, err := service.Find(ctx, projectID, 9); !errors.Is(err, runtimeapp.ErrDeploymentNotFound) {
		t.Errorf("Find of a missing deployment: %v, want ErrDeploymentNotFound", err)
	}
}

func TestDeploymentService_Rollback_PublishesAfterTheCommit(t *testing.T) {
	projectID := uuid.New()
	repo := &fakeDeployments{}
	var published []event.Event
	service := runtimeapp.NewDeploymentService(repo, recordingPublish(&published))
	ctx := context.Background()

	deployment, err := service.Rollback(ctx, projectID, 2)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if !repo.rolledBack || repo.rollbackNumber != 2 {
		t.Errorf("the repository was asked to roll back %d, called %v", repo.rollbackNumber, repo.rolledBack)
	}
	if deployment.Cause != runtimeapp.DeploymentCauseRollback || deployment.RolledBackFrom == nil || *deployment.RolledBackFrom != 2 {
		t.Errorf("the rolled-back deployment: %+v", deployment)
	}
	if len(published) != 1 {
		t.Fatalf("published %d events, want 1", len(published))
	}
	ev, ok := published[0].(runtimeapp.DeploymentCreatedEvent)
	if !ok {
		t.Fatalf("published a %T, not a DeploymentCreatedEvent", published[0])
	}
	if ev.Number() != deployment.Number || ev.ProjectID().UUID() != projectID {
		t.Errorf("the event names deployment %d of project %s", ev.Number(), ev.ProjectID())
	}
}

func TestDeploymentService_Rollback_OfARefusedOne_PublishesNothing(t *testing.T) {
	for name, target := range map[string]error{
		"nothing to rollback": runtimeapp.ErrNothingToRollback,
		"missing deployment":  runtimeapp.ErrDeploymentNotFound,
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeDeployments{rollbackErr: target}
			var published []event.Event
			service := runtimeapp.NewDeploymentService(repo, recordingPublish(&published))

			if _, err := service.Rollback(context.Background(), uuid.New(), 1); !errors.Is(err, target) {
				t.Fatalf("Rollback: %v, want %v", err, target)
			}
			if len(published) != 0 {
				t.Errorf("a refused rollback published %d events", len(published))
			}
		})
	}
}
