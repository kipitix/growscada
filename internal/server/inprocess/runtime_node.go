// Package inprocess wires the roles of the server when they run in one
// process (ADR 0011): the Engineering node's RuntimeNode port over the
// Runtime node's services, called directly. It is the composition of
// main.go (and of servertest), the one place that imports both roles; the
// HTTP implementation of the port comes with separate servers (task 52).
package inprocess

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

type runtimeNode struct {
	states    runtimeapp.TagStateService
	tokens    runtimeapp.DeviceTokenService
	revisions runtimeapp.RevisionService
}

var _ application.RuntimeNode = (*runtimeNode)(nil)

// NewRuntimeNode returns the RuntimeNode calling the Runtime node's services
// of Tag states, of Device Tokens and of Revisions in the same process. A nil
// tokens is a Runtime node that serves no Device Tokens: its removal is a
// no-op; a nil revisions is one that cannot deploy.
func NewRuntimeNode(someStates runtimeapp.TagStateService, someTokens runtimeapp.DeviceTokenService, someRevisions runtimeapp.RevisionService) application.RuntimeNode {
	return &runtimeNode{states: someStates, tokens: someTokens, revisions: someRevisions}
}

func (n runtimeNode) RemoveProjectTagStates(ctx context.Context, projectID uuid.UUID) error {
	return n.states.RemoveProjectTagStates(ctx, projectID)
}

func (n runtimeNode) RemoveProjectDeviceTokens(ctx context.Context, projectID uuid.UUID) error {
	if n.tokens == nil {
		return nil
	}
	return n.tokens.RemoveProjectTokens(ctx, projectID)
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

func (n runtimeNode) Deploy(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (appdto.Revision, error) {
	if n.revisions == nil {
		return appdto.Revision{}, fmt.Errorf("runtime node serves no revisions")
	}
	revision, err := n.revisions.Deploy(ctx, projectID, file)
	if err != nil {
		return appdto.Revision{}, err
	}
	return appdto.Revision{Number: revision.Number, DeployedAt: revision.DeployedAt, Content: revision.Content}, nil
}

func (n runtimeNode) FindCurrentRevision(ctx context.Context, projectID uuid.UUID) (appdto.Revision, bool, error) {
	if n.revisions == nil {
		return appdto.Revision{}, false, fmt.Errorf("runtime node serves no revisions")
	}
	revision, ok, err := n.revisions.FindCurrent(ctx, projectID)
	if err != nil {
		return appdto.Revision{}, false, err
	}
	return appdto.Revision{Number: revision.Number, DeployedAt: revision.DeployedAt, Content: revision.Content}, ok, nil
}
