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
// and handing back the current one.
type fakeRevisions struct {
	created    []uuid.UUID
	documents  []projectv0.ProjectFile
	times      []time.Time
	createErr  error
	current    runtimeapp.Revision
	hasCurrent bool
	findErr    error
}

func (r *fakeRevisions) CreateRevision(_ context.Context, projectID uuid.UUID, file projectv0.ProjectFile, now time.Time) (runtimeapp.Revision, error) {
	if r.createErr != nil {
		return runtimeapp.Revision{}, r.createErr
	}
	r.created = append(r.created, projectID)
	r.documents = append(r.documents, file)
	r.times = append(r.times, now)
	return runtimeapp.Revision{Number: len(r.created), DeployedAt: now, Content: file}, nil
}

func (r *fakeRevisions) FindCurrent(context.Context, uuid.UUID) (runtimeapp.Revision, bool, error) {
	return r.current, r.hasCurrent, r.findErr
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
