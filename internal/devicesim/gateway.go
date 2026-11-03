package devicesim

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
)

// Source is where a Device Gateway reads the configuration of the Device it
// serves: the server's Device API, keyed by the Device Token and the
// Environment the Gateway stands in (ADR 0004, amendment 2026-11-03).
type Source interface {
	// Configuration returns the configuration of the Device in the
	// Environment the Gateway serves. Its error is the request's: a token the
	// server refuses matches devicelink.IsNoPermission.
	Configuration(ctx context.Context) (apiv0.DeviceConfigResponse, error)
	// Events follows the Gateway's configuration events until the stream
	// ends or ctx is done, calling onChange on each. The returned error is
	// that of the end.
	Events(ctx context.Context, onChange func(apiv0.Event)) error
}

// ErrEnvironmentNotFound means the Device cannot be served in the Environment
// the Gateway is configured with: the Project has no Environment of that name
// or ID, or the Device does not stand in it.
var ErrEnvironmentNotFound = errors.New("environment not found")

// Gateway is the Source over the server's REST API: it reads the
// configuration of the Device its token names, in the Environment it serves.
// The Environment may be named by ID, or by name, which is looked up among the
// project's Environments.
type Gateway struct {
	client      *apiclient.Client
	environment string // as configured: an ID or a name

	mu       sync.Mutex
	resolved uuid.UUID // the ID of a name looked up, uuid.Nil until then
}

// NewGateway creates the Source over a client that carries the Gateway's
// Device Token (apiclient.Client.WithDeviceToken).
func NewGateway(client *apiclient.Client, environment string) *Gateway {
	return &Gateway{client: client, environment: environment}
}

// Configuration returns the configuration of the Device in the Gateway's
// Environment. An Environment the Device cannot be served in — one the Project
// does not have, or one it does not stand in — is an error matching
// ErrEnvironmentNotFound.
func (g *Gateway) Configuration(ctx context.Context) (apiv0.DeviceConfigResponse, error) {
	environmentID, byName, err := g.environmentID(ctx)
	if err != nil {
		return apiv0.DeviceConfigResponse{}, err
	}
	config, err := g.client.DeviceConfig(ctx, environmentID)
	if !errors.Is(err, apiclient.ErrNotFound) {
		return config, err
	}
	// The Environment may have been renamed, or the Device taken out of it: an
	// Environment named by a name is looked up once more, the answer for an ID
	// stands. Either way a 404 is a local config the server cannot serve.
	if byName {
		g.forgetEnvironment()
		if environmentID, _, err = g.environmentID(ctx); err != nil {
			return apiv0.DeviceConfigResponse{}, err
		}
		if config, err = g.client.DeviceConfig(ctx, environmentID); !errors.Is(err, apiclient.ErrNotFound) {
			return config, err
		}
	}
	return apiv0.DeviceConfigResponse{}, fmt.Errorf("%w: %q: %w", ErrEnvironmentNotFound, g.environment, err)
}

// Events follows the configuration event stream of the Device.
func (g *Gateway) Events(ctx context.Context, onChange func(apiv0.Event)) error {
	return g.client.DeviceEvents(ctx, onChange)
}

// environmentID returns the ID of the Gateway's Environment and whether it was
// found by name (and so may be forgotten and looked up again).
func (g *Gateway) environmentID(ctx context.Context) (uuid.UUID, bool, error) {
	if id, err := uuid.Parse(g.environment); err == nil {
		return id, false, nil
	}
	g.mu.Lock()
	id := g.resolved
	g.mu.Unlock()
	if id != uuid.Nil {
		return id, true, nil
	}
	identity, err := g.client.DeviceIdentity(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	environments, err := g.client.Project(identity.ProjectID).ListEnvironments(ctx)
	if err != nil {
		return uuid.Nil, false, err
	}
	for _, environment := range environments {
		if environment.Name == g.environment {
			g.mu.Lock()
			g.resolved = environment.ID
			g.mu.Unlock()
			return environment.ID, true, nil
		}
	}
	return uuid.Nil, false, fmt.Errorf("%w: %q", ErrEnvironmentNotFound, g.environment)
}

func (g *Gateway) forgetEnvironment() {
	g.mu.Lock()
	g.resolved = uuid.Nil
	g.mu.Unlock()
}
