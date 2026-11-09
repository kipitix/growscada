package runtime_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	recordv0 "github.com/kipitix/growscada/contract/record/v0"
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

// fakeOperationDeployments is the Runtime node's Deployments as the Operation read
// model sees it.
type fakeOperationDeployments struct {
	runtimeapp.DeploymentService
	deployment runtimeapp.Deployment
	deployed   bool
	err        error
	calls      *[]string
}

func (r fakeOperationDeployments) FindCurrent(_ context.Context, _ uuid.UUID) (runtimeapp.Deployment, bool, error) {
	*r.calls = append(*r.calls, "deployment")
	return r.deployment, r.deployed, r.err
}

// Find is the read GetAt makes: the Deployment a Checkpoint names.
func (r fakeOperationDeployments) Find(_ context.Context, _ uuid.UUID, n int) (runtimeapp.Deployment, error) {
	*r.calls = append(*r.calls, "deployment:"+strconv.Itoa(n))
	if r.err != nil {
		return runtimeapp.Deployment{}, r.err
	}
	return r.deployment, nil
}

// fakeOperationCheckpoints is the Runtime node's Checkpoints as the Operation
// read model sees them.
type fakeOperationCheckpoints struct {
	runtimeapp.CheckpointRepository
	checkpoint recordv0.Checkpoint
	found      bool
	err        error
	calls      *[]string
	asked      *int64
}

func (c fakeOperationCheckpoints) Nearest(_ context.Context, _ uuid.UUID, position int64) (recordv0.Checkpoint, bool, error) {
	*c.calls = append(*c.calls, "checkpoint")
	*c.asked = position
	return c.checkpoint, c.found, c.err
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
func operationService(journal fakeOperationJournal, deployments fakeOperationDeployments, states fakeOperationStates) (runtimeapp.OperationStateService, *[]string) {
	service, calls, _ := operationServiceWithCheckpoints(journal, deployments, states, fakeOperationCheckpoints{})
	return service, calls
}

// operationServiceWithCheckpoints builds the read model over the fakes with the
// Checkpoints the GetAt read takes, recording every read in calls and the
// position the nearest one was asked for.
func operationServiceWithCheckpoints(journal fakeOperationJournal, deployments fakeOperationDeployments, states fakeOperationStates,
	checkpoints fakeOperationCheckpoints) (runtimeapp.OperationStateService, *[]string, *int64) {
	calls := &[]string{}
	asked := new(int64)
	journal.calls, deployments.calls, states.calls = calls, calls, calls
	checkpoints.calls, checkpoints.asked = calls, asked
	return runtimeapp.NewOperationStateService(journal, deployments, states, checkpoints), calls, asked
}

func tagDefinition(id uuid.UUID, name, tagType string) projectv0.TagDefinition {
	return projectv0.TagDefinition{ID: id, Name: name, Type: tagType}
}

func TestOperationStateService_ReadsThePositionThenTheDeploymentThenTheStates(t *testing.T) {
	projectID := uuid.New()
	tagID := uuid.New()
	service, calls := operationService(
		fakeOperationJournal{position: 12},
		fakeOperationDeployments{
			deployed: true,
			deployment: runtimeapp.Deployment{Number: 2, Content: projectv0.ProjectFile{
				Tags: []projectv0.TagDefinition{tagDefinition(tagID, "speed", "integer")},
			}},
		},
		fakeOperationStates{states: []runtimeapp.TagState{{ID: tagID, Value: "3", Quality: "good", Seq: 12}}},
	)

	state, err := service.Get(context.Background(), projectID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := []string{"position", "deployment", "states"}
	if len(*calls) != len(want) {
		t.Fatalf("read %v, want %v", *calls, want)
	}
	for i := range want {
		if (*calls)[i] != want[i] {
			t.Fatalf("read %v, want %v", *calls, want)
		}
	}
	if state.Position != 12 || !state.Deployed || state.Deployment.Number != 2 {
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
				fakeOperationDeployments{deployed: true, deployment: runtimeapp.Deployment{
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

func TestOperationStateService_AProjectWithoutDeploymentAnswersItsPosition(t *testing.T) {
	service, calls := operationService(
		fakeOperationJournal{position: 3},
		fakeOperationDeployments{deployed: false},
		fakeOperationStates{},
	)
	state, err := service.Get(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if state.Position != 3 || state.Deployed || len(state.TagStates) != 0 {
		t.Errorf("state %+v, want position 3 and no deployment", state)
	}
	if len(*calls) != 2 {
		t.Errorf("read %v, want the position and the deployment alone", *calls)
	}
}

func TestOperationStateService_ADeployedProjectWithoutTagsHasNoStates(t *testing.T) {
	service, _ := operationService(
		fakeOperationJournal{position: 9},
		fakeOperationDeployments{deployed: true, deployment: runtimeapp.Deployment{Number: 1}},
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
		fakeOperationDeployments{deployed: true, deployment: runtimeapp.Deployment{
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
		name        string
		journal     fakeOperationJournal
		deployments fakeOperationDeployments
	}{
		{"journal", fakeOperationJournal{err: project.ErrProjectNotFound}, fakeOperationDeployments{deployed: true}},
		{"deployment", fakeOperationJournal{}, fakeOperationDeployments{err: project.ErrProjectNotFound}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _ := operationService(tc.journal, tc.deployments, fakeOperationStates{})
			if _, err := service.Get(context.Background(), projectID); !errors.Is(err, project.ErrProjectNotFound) {
				t.Errorf("%v, want project.ErrProjectNotFound", err)
			}
		})
	}

	service, _ := operationService(
		fakeOperationJournal{},
		fakeOperationDeployments{deployed: true, deployment: runtimeapp.Deployment{
			Number:  1,
			Content: projectv0.ProjectFile{Tags: []projectv0.TagDefinition{tagDefinition(uuid.New(), "speed", "integer")}},
		}},
		fakeOperationStates{err: project.ErrProjectNotFound},
	)
	if _, err := service.Get(context.Background(), projectID); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("a failed Tag-state read: %v, want project.ErrProjectNotFound", err)
	}
}

// TestOperationStateService_GetAtBeforeTheFirstCheckpointIsTheEmptyStateAtZero:
// History has nothing to start from, so it replays the Journal from 0
// (task 44, ADR 0023).
func TestOperationStateService_GetAtBeforeTheFirstCheckpointIsTheEmptyStateAtZero(t *testing.T) {
	service, calls, asked := operationServiceWithCheckpoints(
		fakeOperationJournal{position: 12},
		fakeOperationDeployments{},
		fakeOperationStates{},
		fakeOperationCheckpoints{found: false},
	)
	state, err := service.GetAt(context.Background(), uuid.New(), 8)
	if err != nil {
		t.Fatalf("GetAt: %v", err)
	}
	if state.Position != 0 || state.Deployed || len(state.TagStates) != 0 {
		t.Errorf("state %+v, want the empty state at 0", state)
	}
	if *asked != 8 {
		t.Errorf("asked for the checkpoint at %d, want 8", *asked)
	}
	want := []string{"position", "checkpoint"}
	if len(*calls) != len(want) || (*calls)[0] != want[0] || (*calls)[1] != want[1] {
		t.Errorf("read %v, want %v", *calls, want)
	}
}

// TestOperationStateService_GetAtReadsTheCheckpointsDeploymentAndStates: the
// answer is the snapshot's moment, its Deployment resolved to the document and
// its states as the record keeps them — no written_at, and the definition copy
// from the document (task 44, ADR 0023).
func TestOperationStateService_GetAtReadsTheCheckpointsDeploymentAndStates(t *testing.T) {
	tagID := uuid.New()
	otherID := uuid.New()
	service, calls, asked := operationServiceWithCheckpoints(
		fakeOperationJournal{position: 12},
		fakeOperationDeployments{deployment: runtimeapp.Deployment{
			Number: 2,
			Cause:  runtimeapp.DeploymentCauseDeploy,
			Content: projectv0.ProjectFile{Tags: []projectv0.TagDefinition{
				tagDefinition(tagID, "speed", "integer"),
				tagDefinition(otherID, "gone", "string"),
			}},
		}},
		fakeOperationStates{},
		fakeOperationCheckpoints{
			found: true,
			checkpoint: recordv0.Checkpoint{
				Seq:        10,
				Deployment: &recordv0.Deployment{Number: 2, Cause: recordv0.DeploymentCauseDeploy},
				TagStates: []recordv0.CheckpointTagState{
					{TagID: tagID, Value: "5", Quality: "good", Seq: 9},
					// A Tag the document no longer names: the state is part of
					// the snapshot, its definition copy has no source.
					{TagID: uuid.New(), Value: "x", Quality: "bad", Seq: 3},
				},
			},
		},
	)
	state, err := service.GetAt(context.Background(), uuid.New(), 10)
	if err != nil {
		t.Fatalf("GetAt: %v", err)
	}
	if state.Position != 10 || !state.Deployed || state.Deployment.Number != 2 {
		t.Errorf("state %+v, want position 10 and deployment 2", state)
	}
	if *asked != 10 {
		t.Errorf("asked for the checkpoint at %d, want 10", *asked)
	}
	if len(state.TagStates) != 2 {
		t.Fatalf("tag states %+v, want both of the snapshot", state.TagStates)
	}
	named := state.TagStates[0]
	if named.ID != tagID || named.Name != "speed" || named.Type != "integer" ||
		named.Value != "5" || named.Quality != "good" || named.Seq != 9 || named.WrittenAt != nil {
		t.Errorf("the named state %+v", named)
	}
	unnamed := state.TagStates[1]
	if unnamed.Name != "" || unnamed.Type != "" || unnamed.Value != "x" || unnamed.Quality != "bad" || unnamed.Seq != 3 {
		t.Errorf("the state of a Tag outside the document %+v", unnamed)
	}
	want := []string{"position", "checkpoint", "deployment:2"}
	if len(*calls) != len(want) {
		t.Fatalf("read %v, want %v", *calls, want)
	}
	for i := range want {
		if (*calls)[i] != want[i] {
			t.Fatalf("read %v, want %v", *calls, want)
		}
	}
}

// TestOperationStateService_GetAtBetweenCheckpointsAsksForThePosition: the
// repository takes the nearest one at or before it; the read model passes the
// moment through untouched and answers the snapshot's own seq.
func TestOperationStateService_GetAtBetweenCheckpointsAsksForThePosition(t *testing.T) {
	service, _, asked := operationServiceWithCheckpoints(
		fakeOperationJournal{position: 30},
		fakeOperationDeployments{deployment: runtimeapp.Deployment{Number: 1}},
		fakeOperationStates{},
		fakeOperationCheckpoints{
			found:      true,
			checkpoint: recordv0.Checkpoint{Seq: 20, Deployment: &recordv0.Deployment{Number: 1}},
		},
	)
	state, err := service.GetAt(context.Background(), uuid.New(), 25)
	if err != nil {
		t.Fatalf("GetAt: %v", err)
	}
	if *asked != 25 {
		t.Errorf("asked for the checkpoint at %d, want 25", *asked)
	}
	if state.Position != 20 {
		t.Errorf("position %d, want the checkpoint's 20", state.Position)
	}
}

// TestOperationStateService_GetAtBeyondTheEndIsClamped: a moment past the
// Project's current position is the current one (task 44, ADR 0023).
func TestOperationStateService_GetAtBeyondTheEndIsClamped(t *testing.T) {
	service, _, asked := operationServiceWithCheckpoints(
		fakeOperationJournal{position: 7},
		fakeOperationDeployments{deployment: runtimeapp.Deployment{Number: 1}},
		fakeOperationStates{},
		fakeOperationCheckpoints{
			found:      true,
			checkpoint: recordv0.Checkpoint{Seq: 7, Deployment: &recordv0.Deployment{Number: 1}},
		},
	)
	state, err := service.GetAt(context.Background(), uuid.New(), 99)
	if err != nil {
		t.Fatalf("GetAt: %v", err)
	}
	if *asked != 7 {
		t.Errorf("asked for the checkpoint at %d, want the clamped 7", *asked)
	}
	if state.Position != 7 {
		t.Errorf("position %d, want 7", state.Position)
	}
}

// TestOperationStateService_GetAtANegativePositionIsInvalidInput: the caller's
// input, answered 400, before any read.
func TestOperationStateService_GetAtANegativePositionIsInvalidInput(t *testing.T) {
	service, calls, _ := operationServiceWithCheckpoints(
		fakeOperationJournal{position: 7}, fakeOperationDeployments{}, fakeOperationStates{}, fakeOperationCheckpoints{},
	)
	if _, err := service.GetAt(context.Background(), uuid.New(), -1); !errors.Is(err, runtimeapp.ErrInvalidInput) {
		t.Fatalf("%v, want ErrInvalidInput", err)
	}
	if len(*calls) != 0 {
		t.Errorf("read %v, want no read", *calls)
	}
}

// TestOperationStateService_GetAtWithoutADeployment: a Checkpoint of a Project
// that runs no Deployment answers its position alone.
func TestOperationStateService_GetAtWithoutADeployment(t *testing.T) {
	service, _, _ := operationServiceWithCheckpoints(
		fakeOperationJournal{position: 5}, fakeOperationDeployments{}, fakeOperationStates{},
		fakeOperationCheckpoints{found: true, checkpoint: recordv0.Checkpoint{Seq: 5}},
	)
	state, err := service.GetAt(context.Background(), uuid.New(), 5)
	if err != nil {
		t.Fatalf("GetAt: %v", err)
	}
	if state.Position != 5 || state.Deployed || len(state.TagStates) != 0 {
		t.Errorf("state %+v, want position 5 and no deployment", state)
	}
}

// TestOperationStateService_GetAtPassesTheProjectsFateAndTheReadFailuresThrough.
func TestOperationStateService_GetAtPassesTheProjectsFateAndTheReadFailuresThrough(t *testing.T) {
	projectID := uuid.New()
	for _, tc := range []struct {
		name        string
		journal     fakeOperationJournal
		deployments fakeOperationDeployments
		checkpoints fakeOperationCheckpoints
	}{
		{"journal", fakeOperationJournal{err: project.ErrProjectNotFound}, fakeOperationDeployments{}, fakeOperationCheckpoints{}},
		{"checkpoint", fakeOperationJournal{}, fakeOperationDeployments{}, fakeOperationCheckpoints{err: project.ErrProjectNotFound}},
		{"deployment", fakeOperationJournal{}, fakeOperationDeployments{err: project.ErrProjectNotFound},
			fakeOperationCheckpoints{found: true, checkpoint: recordv0.Checkpoint{Seq: 1, Deployment: &recordv0.Deployment{Number: 1}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _ := operationServiceWithCheckpoints(tc.journal, tc.deployments, fakeOperationStates{}, tc.checkpoints)
			if _, err := service.GetAt(context.Background(), projectID, 1); !errors.Is(err, project.ErrProjectNotFound) {
				t.Errorf("%v, want project.ErrProjectNotFound", err)
			}
		})
	}
}
