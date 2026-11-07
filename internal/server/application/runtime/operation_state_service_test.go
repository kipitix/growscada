package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// fakeOperationJournal is the Runtime node's Journal as the Operation read
// model sees it: what a test gave it, and the calls it was asked to make.
type fakeOperationJournal struct {
	runtimeapp.JournalService
	position int64
	err      error
	calls    *[]string
}

func (j fakeOperationJournal) Position(_ context.Context, _ uuid.UUID) (int64, error) {
	*j.calls = append(*j.calls, "position")
	return j.position, j.err
}

// fakeOperationRevisions is the Runtime node's Revisions as the Operation read
// model sees it.
type fakeOperationRevisions struct {
	runtimeapp.RevisionService
	revision runtimeapp.Revision
	deployed bool
	err      error
	calls    *[]string
}

func (r fakeOperationRevisions) FindCurrent(_ context.Context, _ uuid.UUID) (runtimeapp.Revision, bool, error) {
	*r.calls = append(*r.calls, "revision")
	return r.revision, r.deployed, r.err
}

// fakeOperationStates is the Runtime node's Tag states as the Operation read
// model sees it.
type fakeOperationStates struct {
	runtimeapp.TagStateService
	states []runtimeapp.TagState
	err    error
	calls  *[]string
}

func (s fakeOperationStates) FindTagStates(context.Context, uuid.UUID, []uuid.UUID) ([]runtimeapp.TagState, error) {
	*s.calls = append(*s.calls, "states")
	return s.states, s.err
}

// operationService builds the read model over the fakes, recording every read
// in calls.
func operationService(journal fakeOperationJournal, revisions fakeOperationRevisions, states fakeOperationStates) (runtimeapp.OperationStateService, *[]string) {
	calls := &[]string{}
	journal.calls, revisions.calls, states.calls = calls, calls, calls
	return runtimeapp.NewOperationStateService(journal, revisions, states), calls
}

func tagDefinition(id uuid.UUID, name, tagType string) projectv0.TagDefinition {
	return projectv0.TagDefinition{ID: id, Name: name, Type: tagType}
}

func TestOperationStateService_ReadsThePositionThenTheRevisionThenTheStates(t *testing.T) {
	projectID := uuid.New()
	tagID := uuid.New()
	service, calls := operationService(
		fakeOperationJournal{position: 12},
		fakeOperationRevisions{
			deployed: true,
			revision: runtimeapp.Revision{Number: 2, Content: projectv0.ProjectFile{
				Tags: []projectv0.TagDefinition{tagDefinition(tagID, "speed", "integer")},
			}},
		},
		fakeOperationStates{states: []runtimeapp.TagState{{ID: tagID, Value: "3", Quality: "good", Seq: 12}}},
	)

	state, err := service.Get(context.Background(), projectID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := []string{"position", "revision", "states"}
	if len(*calls) != len(want) {
		t.Fatalf("read %v, want %v", *calls, want)
	}
	for i := range want {
		if (*calls)[i] != want[i] {
			t.Fatalf("read %v, want %v", *calls, want)
		}
	}
	if state.Position != 12 || !state.Deployed || state.Revision.Number != 2 {
		t.Errorf("state %+v", state)
	}
	if len(state.TagStates) != 1 || state.TagStates[0].Value != "3" || state.TagStates[0].Seq != 12 {
		t.Errorf("tag states %+v", state.TagStates)
	}
}

func TestOperationStateService_AnUnwrittenTagIsItsZeroValueWithQualityBad(t *testing.T) {
	writtenAt := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	given := map[string]string{"id": "string", "integer": "integer", "boolean": "boolean"}
	zero := map[string]string{"string": "", "integer": "0", "boolean": "false"}
	for name, tagType := range given {
		t.Run(name, func(t *testing.T) {
			tagID := uuid.New()
			writtenID := uuid.New()
			service, _ := operationService(
				fakeOperationJournal{position: 5},
				fakeOperationRevisions{deployed: true, revision: runtimeapp.Revision{
					Number: 1,
					Content: projectv0.ProjectFile{Tags: []projectv0.TagDefinition{
						tagDefinition(writtenID, "written", "integer"),
						{ID: tagID, Name: "unwritten", Type: tagType},
					}},
				}},
				fakeOperationStates{states: []runtimeapp.TagState{
					{ID: writtenID, Name: "written", Type: "integer", Value: "7", Quality: "good", Seq: 4, WrittenAt: &writtenAt},
				}},
			)

			state, err := service.Get(context.Background(), uuid.New())
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if len(state.TagStates) != 2 {
				t.Fatalf("states %+v, want both tags", state.TagStates)
			}
			if state.TagStates[0].ID != writtenID || state.TagStates[0].Seq != 4 {
				t.Errorf("the written tag %+v", state.TagStates[0])
			}
			unwritten := state.TagStates[1]
			if unwritten.ID != tagID || unwritten.Name != "unwritten" || unwritten.Type != tagType {
				t.Errorf("the unwritten tag keeps no definition copy: %+v", unwritten)
			}
			if unwritten.Value != zero[name] || unwritten.Quality != "bad" || unwritten.Seq != 0 || unwritten.WrittenAt != nil {
				t.Errorf("the unwritten %s tag is %+v, want %q/bad/0 without written_at", name, unwritten, zero[name])
			}
		})
	}
}

func TestOperationStateService_AProjectWithoutRevisionAnswersItsPosition(t *testing.T) {
	service, calls := operationService(
		fakeOperationJournal{position: 3},
		fakeOperationRevisions{deployed: false},
		fakeOperationStates{},
	)
	state, err := service.Get(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if state.Position != 3 || state.Deployed || len(state.TagStates) != 0 {
		t.Errorf("state %+v, want position 3 and no revision", state)
	}
	if len(*calls) != 2 {
		t.Errorf("read %v, want the position and the revision alone", *calls)
	}
}

func TestOperationStateService_ADeployedProjectWithoutTagsHasNoStates(t *testing.T) {
	service, _ := operationService(
		fakeOperationJournal{position: 9},
		fakeOperationRevisions{deployed: true, revision: runtimeapp.Revision{Number: 1}},
		fakeOperationStates{},
	)
	state, err := service.Get(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if state.Position != 9 || !state.Deployed || len(state.TagStates) != 0 {
		t.Errorf("state %+v, want position 9 and no tag states", state)
	}
}

func TestOperationStateService_RefusesADocumentWithAnUnknownTagType(t *testing.T) {
	service, _ := operationService(
		fakeOperationJournal{},
		fakeOperationRevisions{deployed: true, revision: runtimeapp.Revision{
			Number:  1,
			Content: projectv0.ProjectFile{Tags: []projectv0.TagDefinition{tagDefinition(uuid.New(), "speed", "float")}},
		}},
		fakeOperationStates{},
	)
	if _, err := service.Get(context.Background(), uuid.New()); err == nil {
		t.Fatal("Get of a document with an unknown tag type: no error")
	}
}

func TestOperationStateService_PassesTheProjectsFateAndTheReadFailuresThrough(t *testing.T) {
	projectID := uuid.New()
	for _, tc := range []struct {
		name      string
		journal   fakeOperationJournal
		revisions fakeOperationRevisions
	}{
		{"journal", fakeOperationJournal{err: project.ErrProjectNotFound}, fakeOperationRevisions{deployed: true}},
		{"revision", fakeOperationJournal{}, fakeOperationRevisions{err: project.ErrProjectNotFound}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _ := operationService(tc.journal, tc.revisions, fakeOperationStates{})
			if _, err := service.Get(context.Background(), projectID); !errors.Is(err, project.ErrProjectNotFound) {
				t.Errorf("%v, want project.ErrProjectNotFound", err)
			}
		})
	}

	service, _ := operationService(
		fakeOperationJournal{},
		fakeOperationRevisions{deployed: true, revision: runtimeapp.Revision{
			Number:  1,
			Content: projectv0.ProjectFile{Tags: []projectv0.TagDefinition{tagDefinition(uuid.New(), "speed", "integer")}},
		}},
		fakeOperationStates{err: project.ErrProjectNotFound},
	)
	if _, err := service.Get(context.Background(), projectID); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("a failed Tag-state read: %v, want project.ErrProjectNotFound", err)
	}
}
