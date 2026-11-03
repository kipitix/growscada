package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// DeviceConfigService serves a Device Gateway the configuration of its Device
// in one Environment (ADR 0004, amendment 2026-11-01): what the Device is,
// where it stands, how it is reached there and which Tags it supplies. The
// Device is named by the token and the Environment by the request; both must
// be of one Project.
//
// Until Deploy exists (task 35) the configuration is read from the Draft, not
// from a Revision.
type DeviceConfigService interface {
	// Config returns the configuration of the Device in the Environment.
	// Returns an error wrapping device.ErrDeviceNotFound if the Project has
	// no such Device (a dormant token), and one wrapping
	// device.ErrEnvironmentNotFound or device.ErrEnvironmentDeviceNotFound
	// if it has no such Environment or the Device does not stand in it.
	Config(ctx context.Context, projectID, deviceID, environmentID uuid.UUID) (appdto.DeviceConfig, error)
}

type deviceConfigServiceImpl struct {
	content ProjectContent
}

var _ DeviceConfigService = (*deviceConfigServiceImpl)(nil)

// NewDeviceConfigService returns the service of the Gateways' configuration.
func NewDeviceConfigService(aContent ProjectContent) DeviceConfigService {
	return &deviceConfigServiceImpl{content: aContent}
}

func (s deviceConfigServiceImpl) Config(ctx context.Context, rawProjectID, rawDeviceID, rawEnvironmentID uuid.UUID) (appdto.DeviceConfig, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.DeviceConfig{}, err
	}
	foundDevice, err := scope.Devices().FindByID(ctx, id.NewID(id.IDWithUUID[device.Device](rawDeviceID)))
	if err != nil {
		return appdto.DeviceConfig{}, fmt.Errorf("error on find the gateway's device in repository: %w", err)
	}
	foundEnvironment, err := scope.Environments().FindByID(ctx, id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID)))
	if err != nil {
		return appdto.DeviceConfig{}, fmt.Errorf("error on find the gateway's environment in repository: %w", err)
	}
	environmentID := id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID))
	deviceID := id.NewID(id.IDWithUUID[device.Device](rawDeviceID))
	pair, err := pairOfProject(ctx, scope.EnvironmentDevices(), environmentID, deviceID)
	if err != nil {
		return appdto.DeviceConfig{}, err
	}
	tags, err := scope.Tags().FindAll(ctx)
	if err != nil {
		return appdto.DeviceConfig{}, fmt.Errorf("error on find the device's tags in repository: %w", err)
	}
	supplied := make([]appdto.DeviceConfigTag, 0, len(tags))
	for _, t := range tags {
		if !t.Supplier().HasDevice() || t.Supplier().DeviceID() != deviceID {
			continue
		}
		supplied = append(supplied, appdto.DeviceConfigTag{
			ID:   t.ID().UUID(),
			Name: t.Name().String(),
			Type: t.Type().String(),
		})
	}
	return appdto.DeviceConfig{
		Device:      appdto.NewDevice(foundDevice),
		Environment: appdto.NewEnvironment(foundEnvironment),
		Connection:  pair.Params().Bytes(),
		Tags:        supplied,
	}, nil
}
