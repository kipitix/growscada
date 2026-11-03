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
	defs      []runtimeapp.TagDefinition
	tagIDs    []uuid.UUID
	removeAll bool
	found     []runtimeapp.TagState
	err       error
}

func (s *recordingStates) UpsertTagDefinitions(_ context.Context, projectID uuid.UUID, defs []runtimeapp.TagDefinition) error {
	s.projectID, s.defs = projectID, defs
	return s.err
}

func (s *recordingStates) RemoveTagStates(_ context.Context, projectID uuid.UUID, tagIDs []uuid.UUID) error {
	s.projectID, s.tagIDs = projectID, tagIDs
	return s.err
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

func TestRuntimeNode_UpsertTagDefinitions_HandsOverEveryField(t *testing.T) {
	states := &recordingStates{}
	project := uuid.New()
	first, second := uuid.New(), uuid.New()

	err := inprocess.NewRuntimeNode(states, nil, nil).UpsertTagDefinitions(context.Background(), project, []appdto.TagDefinition{
		{ID: first, Name: "pressure", Type: "integer", Version: 1},
		{ID: second, Name: "running", Type: "boolean", Version: 3},
	})
	if err != nil {
		t.Fatalf("UpsertTagDefinitions: %v", err)
	}

	want := []runtimeapp.TagDefinition{
		{ID: first, Name: "pressure", Type: "integer", Version: 1},
		{ID: second, Name: "running", Type: "boolean", Version: 3},
	}
	if states.projectID != project || !reflect.DeepEqual(states.defs, want) {
		t.Errorf("upserted %v of project %v, want %v of %v", states.defs, states.projectID, want, project)
	}
}

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

func TestRuntimeNode_RemoveTagStates_HandsOverTheTags(t *testing.T) {
	states := &recordingStates{}
	project, tagID := uuid.New(), uuid.New()

	if err := inprocess.NewRuntimeNode(states, nil, nil).RemoveTagStates(context.Background(), project, []uuid.UUID{tagID}); err != nil {
		t.Fatalf("RemoveTagStates: %v", err)
	}
	if states.projectID != project || !reflect.DeepEqual(states.tagIDs, []uuid.UUID{tagID}) {
		t.Errorf("removed %v of project %v, want [%v] of %v", states.tagIDs, states.projectID, tagID, project)
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
		"UpsertTagDefinitions":   node.UpsertTagDefinitions(ctx, project, []appdto.TagDefinition{{ID: uuid.New()}}),
		"RemoveTagStates":        node.RemoveTagStates(ctx, project, []uuid.UUID{uuid.New()}),
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

// recordingRevisions is a RevisionService that records what it is given and
// answers with revision, or err.
type recordingRevisions struct {
	runtimeapp.RevisionService

	projectID  uuid.UUID
	file       projectv0.ProjectFile
	revision   runtimeapp.Revision
	current    runtimeapp.Revision
	hasCurrent bool
	err        error
}

func (r *recordingRevisions) Deploy(_ context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (runtimeapp.Revision, error) {
	r.projectID, r.file = projectID, file
	return r.revision, r.err
}

func (r *recordingRevisions) FindCurrent(context.Context, uuid.UUID) (runtimeapp.Revision, bool, error) {
	return r.current, r.hasCurrent, r.err
}

// TestRuntimeNode_Deploy_HandsOverTheDocumentAndConvertsTheRevision: the
// Engineering node's DTO of a Revision is the Runtime node's.
func TestRuntimeNode_Deploy_HandsOverTheDocumentAndConvertsTheRevision(t *testing.T) {
	projectID := uuid.New()
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	file := projectv0.ProjectFile{Header: projectv0.Header{ProjectID: projectID}}
	revisions := &recordingRevisions{revision: runtimeapp.Revision{Number: 3, DeployedAt: at, Content: file}}

	got, err := inprocess.NewRuntimeNode(&recordingStates{}, nil, revisions).Deploy(context.Background(), projectID, file)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if revisions.projectID != projectID || !reflect.DeepEqual(revisions.file, file) {
		t.Errorf("deployed %v of %v, want %v of %v", revisions.file, revisions.projectID, file, projectID)
	}
	want := appdto.Revision{Number: 3, DeployedAt: at, Content: file}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Deploy = %+v, want %+v", got, want)
	}
}

// TestRuntimeNode_FindCurrentRevision_ConvertsTheRevisionAndItsPresence.
func TestRuntimeNode_FindCurrentRevision_ConvertsTheRevisionAndItsPresence(t *testing.T) {
	projectID := uuid.New()
	at := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	file := projectv0.ProjectFile{Header: projectv0.Header{ProjectID: projectID}}
	revisions := &recordingRevisions{current: runtimeapp.Revision{Number: 2, DeployedAt: at, Content: file}, hasCurrent: true}

	got, ok, err := inprocess.NewRuntimeNode(&recordingStates{}, nil, revisions).FindCurrentRevision(context.Background(), projectID)
	if err != nil || !ok {
		t.Fatalf("FindCurrentRevision: %+v, %t, %v", got, ok, err)
	}
	want := appdto.Revision{Number: 2, DeployedAt: at, Content: file}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindCurrentRevision = %+v, want %+v", got, want)
	}

	revisions.hasCurrent = false
	if _, ok, err := inprocess.NewRuntimeNode(&recordingStates{}, nil, revisions).FindCurrentRevision(context.Background(), projectID); err != nil || ok {
		t.Errorf("a project without a revision: %t, %v", ok, err)
	}
}

// TestRuntimeNode_WithoutRevisions_RefusesToDeploy: a Runtime node wired
// without the Revision service has nothing to deploy to.
func TestRuntimeNode_WithoutRevisions_RefusesToDeploy(t *testing.T) {
	node := inprocess.NewRuntimeNode(&recordingStates{}, nil, nil)
	if _, err := node.Deploy(context.Background(), uuid.New(), projectv0.ProjectFile{}); err == nil {
		t.Error("Deploy without a Revision service returned no error")
	}
	if _, _, err := node.FindCurrentRevision(context.Background(), uuid.New()); err == nil {
		t.Error("FindCurrentRevision without a Revision service returned no error")
	}
}
