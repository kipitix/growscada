package inprocess_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

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

	err := inprocess.NewRuntimeNode(states).UpsertTagDefinitions(context.Background(), project, []appdto.TagDefinition{
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

	got, err := inprocess.NewRuntimeNode(states).FindTagStates(context.Background(), project, []uuid.UUID{second, first})
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

	if err := inprocess.NewRuntimeNode(states).RemoveTagStates(context.Background(), project, []uuid.UUID{tagID}); err != nil {
		t.Fatalf("RemoveTagStates: %v", err)
	}
	if states.projectID != project || !reflect.DeepEqual(states.tagIDs, []uuid.UUID{tagID}) {
		t.Errorf("removed %v of project %v, want [%v] of %v", states.tagIDs, states.projectID, tagID, project)
	}
}

func TestRuntimeNode_RemoveProjectTagStates_HandsOverTheProject(t *testing.T) {
	states := &recordingStates{}
	project := uuid.New()

	if err := inprocess.NewRuntimeNode(states).RemoveProjectTagStates(context.Background(), project); err != nil {
		t.Fatalf("RemoveProjectTagStates: %v", err)
	}
	if !states.removeAll || states.projectID != project {
		t.Errorf("removed all: %v of project %v, want true of %v", states.removeAll, states.projectID, project)
	}
}

// TestRuntimeNode_ReturnsTheRuntimeNodesErrors: the Engineering node sees
// the Runtime node's error itself (errors.Is), not a translation of it.
func TestRuntimeNode_ReturnsTheRuntimeNodesErrors(t *testing.T) {
	node := inprocess.NewRuntimeNode(&recordingStates{err: errRuntime})
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
