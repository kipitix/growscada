package runtime

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// DeviceConfigService serves a Device Gateway the configuration of its Device
// in one Environment (ADR 0004, amendment 2026-11-01): what the Device is,
// where it stands, how it is reached there and which Tags it supplies. The
// Device is named by the token and the Environment by the request; both must
// be of one Project.
//
// The configuration is the running Deployment's: the copies of the Runtime node,
// never the Draft (task 35, Q5).
type DeviceConfigService interface {
	// Config returns the configuration of the Device in the Environment.
	// Returns an error wrapping device.ErrDeviceNotFound if the Deployment has
	// no such Device (a dormant token), and one wrapping
	// device.ErrEnvironmentNotFound or device.ErrEnvironmentDeviceNotFound
	// if it has no such Environment or the Device does not stand in it.
	Config(ctx context.Context, projectID, deviceID, environmentID uuid.UUID) (DeviceConfig, error)
}

// DeviceConfigRepository reads the configuration of a Device from the
// Runtime node's copies, which the running Deployment fills.
type DeviceConfigRepository interface {
	// FindConfig returns the configuration of the Device in the Environment,
	// read in one transaction. A Device the Deployment does not have is
	// device.ErrDeviceNotFound; an Environment or a pair it does not have is
	// device.ErrEnvironmentNotFound or device.ErrEnvironmentDeviceNotFound.
	FindConfig(ctx context.Context, projectID, deviceID, environmentID uuid.UUID) (DeviceConfig, error)
}

// DeviceConfig is what a Device Gateway needs to run one Device in one
// Environment: the Device itself, the Environment it stands in, the connection
// parameters of the pair and the Tags the Device supplies.
type DeviceConfig struct {
	Device      DeviceConfigDevice
	Environment DeviceConfigEnvironment
	// Connection is the raw parameters of the pair (Environment, Device).
	Connection json.RawMessage
	Tags       []DeviceConfigTag
}

// DeviceConfigDevice is the Device of a configuration: its protocol and the
// configuration read under it.
type DeviceConfigDevice struct {
	ID       uuid.UUID
	Name     string
	Protocol string
	Config   json.RawMessage
}

// DeviceConfigEnvironment is the Environment of a configuration.
type DeviceConfigEnvironment struct {
	ID   uuid.UUID
	Name string
}

// DeviceConfigTag is a Tag the Gateway writes: its definition's identity,
// name and type.
type DeviceConfigTag struct {
	ID   uuid.UUID
	Name string
	Type string
}

type deviceConfigServiceImpl struct {
	copies DeviceConfigRepository
}

var _ DeviceConfigService = (*deviceConfigServiceImpl)(nil)

// NewDeviceConfigService returns the service of the Gateways' configuration
// over the Runtime node's copies.
func NewDeviceConfigService(copies DeviceConfigRepository) DeviceConfigService {
	return &deviceConfigServiceImpl{copies: copies}
}

func (s deviceConfigServiceImpl) Config(ctx context.Context, projectID, deviceID, environmentID uuid.UUID) (DeviceConfig, error) {
	return s.copies.FindConfig(ctx, projectID, deviceID, environmentID)
}
