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
	states      runtimeapp.TagStateService
	tokens      runtimeapp.DeviceTokenService
	deployments runtimeapp.DeploymentService
}

var _ application.RuntimeNode = (*runtimeNode)(nil)

// NewRuntimeNode returns the RuntimeNode calling the Runtime node's services
// of Tag states, of Device Tokens and of Deployments in the same process. A nil
// tokens is a Runtime node that serves no Device Tokens: its removal is a
// no-op; a nil deployments is one that cannot deploy.
func NewRuntimeNode(someStates runtimeapp.TagStateService, someTokens runtimeapp.DeviceTokenService, someDeployments runtimeapp.DeploymentService) application.RuntimeNode {
	return &runtimeNode{states: someStates, tokens: someTokens, deployments: someDeployments}
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
		states[i] = appdto.TagState{ID: s.ID, Value: s.Value, Quality: s.Quality, Seq: s.Seq, WrittenAt: s.WrittenAt}
	}
	return states, nil
}

func (n runtimeNode) Deploy(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (appdto.Deployment, error) {
	if n.deployments == nil {
		return appdto.Deployment{}, fmt.Errorf("runtime node serves no deployments")
	}
	deployment, err := n.deployments.Deploy(ctx, projectID, file)
	if err != nil {
		return appdto.Deployment{}, err
	}
	return toAppdtoDeployment(deployment), nil
}

func (n runtimeNode) DeployFile(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (appdto.Deployment, error) {
	if n.deployments == nil {
		return appdto.Deployment{}, fmt.Errorf("runtime node serves no deployments")
	}
	deployment, err := n.deployments.DeployFile(ctx, projectID, file)
	if err != nil {
		return appdto.Deployment{}, err
	}
	return toAppdtoDeployment(deployment), nil
}

func (n runtimeNode) FindDeployment(ctx context.Context, projectID uuid.UUID, number int) (appdto.Deployment, error) {
	if n.deployments == nil {
		return appdto.Deployment{}, fmt.Errorf("runtime node serves no deployments")
	}
	deployment, err := n.deployments.Find(ctx, projectID, number)
	if err != nil {
		return appdto.Deployment{}, err
	}
	return toAppdtoDeployment(deployment), nil
}

func (n runtimeNode) FindCurrentDeployment(ctx context.Context, projectID uuid.UUID) (appdto.Deployment, bool, error) {
	if n.deployments == nil {
		return appdto.Deployment{}, false, fmt.Errorf("runtime node serves no deployments")
	}
	deployment, ok, err := n.deployments.FindCurrent(ctx, projectID)
	if err != nil {
		return appdto.Deployment{}, false, err
	}
	return toAppdtoDeployment(deployment), ok, nil
}

// toAppdtoDeployment is the Runtime node's Deployment as the Engineering node's
// appdto.Deployment: the roles keep their own types, and the composition in one
// process is where the two meet (ADR 0011).
func toAppdtoDeployment(deployment runtimeapp.Deployment) appdto.Deployment {
	return appdto.Deployment{
		Number:         deployment.Number,
		DeployedAt:     deployment.DeployedAt,
		Cause:          string(deployment.Cause),
		RolledBackFrom: deployment.RolledBackFrom,
		Content:        deployment.Content,
	}
}
