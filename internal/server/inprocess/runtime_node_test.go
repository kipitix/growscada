package inprocess_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/inprocess"
)

// recordingStates is a TagStateService that records what it is called with
// and answers FindTagStates with found.
type recordingStates struct {
	runtimeapp.TagStateService

	projectID uuid.UUID
	tagIDs    []uuid.UUID
	removeAll bool
	found     []runtimeapp.TagState
	err       error
}

func (s *recordingStates) RemoveProjectTagStates(_ context.Context, projectID uuid.UUID) error {
	s.projectID, s.removeAll = projectID, true
	return s.err
}

func (s *recordingStates) FindTagStates(_ context.Context, projectID uuid.UUID, tagIDs []uuid.UUID) ([]runtimeapp.TagState, error) {
	s.projectID, s.tagIDs = projectID, tagIDs
	return s.found, s.err
}

var errRuntime = errors.New("runtime node down")

func TestRuntimeNode_FindTagStates_HandsOverTheStatesInOrder(t *testing.T) {
	project := uuid.New()
	first, second := uuid.New(), uuid.New()
	states := &recordingStates{found: []runtimeapp.TagState{
		{ID: second, Name: "running", Type: "boolean", Version: 3, Value: "true", Quality: "good", Seq: 42},
		{ID: first, Name: "pressure", Type: "integer", Version: 1, Value: "0", Quality: "bad"},
	}}

	got, err := inprocess.NewRuntimeNode(states, nil, nil).FindTagStates(context.Background(), project, []uuid.UUID{second, first})
	if err != nil {
		t.Fatalf("FindTagStates: %v", err)
	}

	want := []appdto.TagState{
		{ID: second, Value: "true", Quality: "good", Seq: 42},
		{ID: first, Value: "0", Quality: "bad"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("states %v, want %v", got, want)
	}
	if states.projectID != project || !reflect.DeepEqual(states.tagIDs, []uuid.UUID{second, first}) {
		t.Errorf("asked for %v of project %v, want %v of %v", states.tagIDs, states.projectID, []uuid.UUID{second, first}, project)
	}
}

func TestRuntimeNode_RemoveProjectTagStates_HandsOverTheProject(t *testing.T) {
	states := &recordingStates{}
	project := uuid.New()

	if err := inprocess.NewRuntimeNode(states, nil, nil).RemoveProjectTagStates(context.Background(), project); err != nil {
		t.Fatalf("RemoveProjectTagStates: %v", err)
	}
	if !states.removeAll || states.projectID != project {
		t.Errorf("removed all: %v of project %v, want true of %v", states.removeAll, states.projectID, project)
	}
}

// TestRuntimeNode_RemoveProjectDeviceTokens_HandsOverTheProject: the removal
// of a Project's Device Tokens is the Runtime node's, so a token resolved in
// its memory is forgotten with the rows.
type recordingTokens struct {
	runtimeapp.DeviceTokenService

	projectID uuid.UUID
	err       error
}

func (t *recordingTokens) RemoveProjectTokens(_ context.Context, projectID uuid.UUID) error {
	t.projectID = projectID
	return t.err
}

func TestRuntimeNode_RemoveProjectDeviceTokens_HandsOverTheProject(t *testing.T) {
	tokens := &recordingTokens{}
	project := uuid.New()

	if err := inprocess.NewRuntimeNode(&recordingStates{}, tokens, nil).RemoveProjectDeviceTokens(context.Background(), project); err != nil {
		t.Fatalf("RemoveProjectDeviceTokens: %v", err)
	}
	if tokens.projectID != project {
		t.Errorf("forgot the tokens of %v, want %v", tokens.projectID, project)
	}
}

// TestRuntimeNode_RemoveProjectDeviceTokens_WithoutTokens: a Runtime node
// without Device Tokens has nothing to forget.
func TestRuntimeNode_RemoveProjectDeviceTokens_WithoutTokens(t *testing.T) {
	if err := inprocess.NewRuntimeNode(&recordingStates{}, nil, nil).RemoveProjectDeviceTokens(context.Background(), uuid.New()); err != nil {
		t.Errorf("RemoveProjectDeviceTokens without tokens: %v", err)
	}
}

// TestRuntimeNode_ReturnsTheRuntimeNodesErrors: the Engineering node sees
// the Runtime node's error itself (errors.Is), not a translation of it.
func TestRuntimeNode_ReturnsTheRuntimeNodesErrors(t *testing.T) {
	node := inprocess.NewRuntimeNode(&recordingStates{err: errRuntime}, nil, nil)
	ctx, project := context.Background(), uuid.New()

	calls := map[string]error{
		"RemoveProjectTagStates": node.RemoveProjectTagStates(ctx, project),
	}
	states, err := node.FindTagStates(ctx, project, []uuid.UUID{uuid.New()})
	calls["FindTagStates"] = err
	if states != nil {
		t.Errorf("FindTagStates returned %v with its error", states)
	}
	for name, err := range calls {
		if !errors.Is(err, errRuntime) {
			t.Errorf("%s: error %v, want the Runtime node's", name, err)
		}
	}
}

// recordingDeployments is a DeploymentService that records what it is given and
// answers with deployment, or err.
type recordingDeployments struct {
	runtimeapp.DeploymentService

	projectID  uuid.UUID
	file       projectv0.ProjectFile
	release    int
	deployment runtimeapp.Deployment
	current    runtimeapp.Deployment
	hasCurrent bool
	err        error
}

func (r *recordingDeployments) DeployRelease(_ context.Context, projectID uuid.UUID, releaseNumber int, file projectv0.ProjectFile) (runtimeapp.Deployment, error) {
	r.projectID, r.file, r.release = projectID, file, releaseNumber
	return r.deployment, r.err
}

func (r *recordingDeployments) FindCurrent(context.Context, uuid.UUID) (runtimeapp.Deployment, bool, error) {
	return r.current, r.hasCurrent, r.err
}

// TestRuntimeNode_DeployRelease_HandsOverTheReleaseAndConvertsTheDeployment:
// the Engineering node's DTO of a Deployment is the Runtime node's.
func TestRuntimeNode_DeployRelease_HandsOverTheReleaseAndConvertsTheDeployment(t *testing.T) {
	projectID := uuid.New()
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	file := projectv0.ProjectFile{Header: projectv0.Header{ProjectID: projectID}}
	deployments := &recordingDeployments{deployment: runtimeapp.Deployment{Number: 3, DeployedAt: at, Content: file}}

	got, err := inprocess.NewRuntimeNode(&recordingStates{}, nil, deployments).DeployRelease(context.Background(), projectID, 4, file)
	if err != nil {
		t.Fatalf("DeployRelease: %v", err)
	}
	if deployments.projectID != projectID || deployments.release != 4 || !reflect.DeepEqual(deployments.file, file) {
		t.Errorf("deployed %v of %v, want %v of %v", deployments.file, deployments.projectID, file, projectID)
	}
	want := appdto.Deployment{Number: 3, DeployedAt: at, Content: file}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Deploy = %+v, want %+v", got, want)
	}
}

// TestRuntimeNode_FindCurrentDeployment_ConvertsTheDeploymentAndItsPresence.
func TestRuntimeNode_FindCurrentDeployment_ConvertsTheDeploymentAndItsPresence(t *testing.T) {
	projectID := uuid.New()
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	file := projectv0.ProjectFile{Header: projectv0.Header{ProjectID: projectID}}
	deployments := &recordingDeployments{current: runtimeapp.Deployment{Number: 2, DeployedAt: at, Content: file}, hasCurrent: true}

	got, ok, err := inprocess.NewRuntimeNode(&recordingStates{}, nil, deployments).FindCurrentDeployment(context.Background(), projectID)
	if err != nil || !ok {
		t.Fatalf("FindCurrentDeployment: %+v, %t, %v", got, ok, err)
	}
	want := appdto.Deployment{Number: 2, DeployedAt: at, Content: file}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindCurrentDeployment = %+v, want %+v", got, want)
	}

	deployments.hasCurrent = false
	if _, ok, err := inprocess.NewRuntimeNode(&recordingStates{}, nil, deployments).FindCurrentDeployment(context.Background(), projectID); err != nil || ok {
		t.Errorf("a project without a deployment: %t, %v", ok, err)
	}
}

// TestRuntimeNode_WithoutDeployments_RefusesToDeploy: a Runtime node wired
// without the Deployment service has nothing to deploy to.
func TestRuntimeNode_WithoutDeployments_RefusesToDeploy(t *testing.T) {
	node := inprocess.NewRuntimeNode(&recordingStates{}, nil, nil)
	if _, err := node.DeployRelease(context.Background(), uuid.New(), 1, projectv0.ProjectFile{}); err == nil {
		t.Error("DeployRelease without a Deployment service returned no error")
	}
	if _, _, err := node.FindCurrentDeployment(context.Background(), uuid.New()); err == nil {
		t.Error("FindCurrentDeployment without a Deployment service returned no error")
	}
}
