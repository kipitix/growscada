// Package inprocess wires the roles of the server when they run in one
// process (ADR 0011): the Engineering node's RuntimeNode port over the
// Runtime node's services, called directly. It is the composition of
// main.go (and of servertest), the one place that imports both roles; the
// HTTP implementation of the port comes with separate servers (task 52).
package inprocess

import (
	"context"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

type runtimeNode struct {
	states runtimeapp.TagStateService
}

var _ application.RuntimeNode = (*runtimeNode)(nil)

// NewRuntimeNode returns the RuntimeNode calling the Runtime node's service
// of Tag states in the same process.
func NewRuntimeNode(someStates runtimeapp.TagStateService) application.RuntimeNode {
	return &runtimeNode{states: someStates}
}

func (n runtimeNode) UpsertTagDefinitions(ctx context.Context, projectID uuid.UUID, defs []appdto.TagDefinition) error {
	converted := make([]runtimeapp.TagDefinition, len(defs))
	for i, d := range defs {
		converted[i] = runtimeapp.TagDefinition{ID: d.ID, Name: d.Name, Type: d.Type, Version: d.Version}
	}
	return n.states.UpsertTagDefinitions(ctx, projectID, converted)
}

func (n runtimeNode) RemoveTagStates(ctx context.Context, projectID uuid.UUID, tagIDs []uuid.UUID) error {
	return n.states.RemoveTagStates(ctx, projectID, tagIDs)
}

func (n runtimeNode) RemoveProjectTagStates(ctx context.Context, projectID uuid.UUID) error {
	return n.states.RemoveProjectTagStates(ctx, projectID)
}

func (n runtimeNode) FindTagStates(ctx context.Context, projectID uuid.UUID, tagIDs []uuid.UUID) ([]appdto.TagState, error) {
	found, err := n.states.FindTagStates(ctx, projectID, tagIDs)
	if err != nil {
		return nil, err
	}
	states := make([]appdto.TagState, len(found))
	for i, s := range found {
		states[i] = appdto.TagState{ID: s.ID, Value: s.Value, Quality: s.Quality, Seq: s.Seq}
	}
	return states, nil
}
