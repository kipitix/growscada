package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// DeviceService is the service interface for working with the Devices of a
// Project's Draft: what its Tags may be supplied by (ADR 0004, amendment
// 2026-11-01). The parameters by which a Device is reached in an Environment
// are the EnvironmentService's. Every method takes the project first; a
// project that does not exist (or is deleted) is an error wrapping
// project.ErrProjectNotFound.
type DeviceService interface {
	FindAllDevices(ctx context.Context, projectID uuid.UUID) ([]appdto.Device, error)
	FindDeviceByID(ctx context.Context, projectID, deviceID uuid.UUID) (appdto.Device, error)
	FindDeviceByName(ctx context.Context, projectID uuid.UUID, name string) (appdto.Device, error)
	CreateDevice(ctx context.Context, projectID uuid.UUID, input appdto.DeviceInput) (appdto.Device, error)
	// UpdateDevice changes a Device, expecting its Version.
	UpdateDevice(ctx context.Context, projectID, deviceID uuid.UUID, version int, input appdto.DeviceInput) (appdto.Device, error)
	// DeleteDeviceByID removes a Device, expecting no Version; a Device a Tag
	// is supplied by, or one that stands in an Environment, is not removed
	// (device.ErrDeviceInUse).
	DeleteDeviceByID(ctx context.Context, projectID, deviceID uuid.UUID) (appdto.Device, error)
}

type deviceServiceImpl struct {
	content ProjectContent
}

var _ DeviceService = (*deviceServiceImpl)(nil)

// NewDeviceService returns the service of the Devices. The events of its
// changes are recorded by the Device and saved with it (ADR 0008).
func NewDeviceService(aContent ProjectContent) DeviceService {
	return &deviceServiceImpl{content: aContent}
}

// FindAllDevices returns all Devices of the Project. The UI reloads the list
// on every event, so the Devices are read with the check of their Project in
// one query, and the Project is read apart only when there are none, to tell
// a missing Project from an empty one.
func (s deviceServiceImpl) FindAllDevices(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.Device, error) {
	_, scope := s.content.openUnchecked(rawProjectID)
	list, err := scope.Devices().FindAllInLiveProject(ctx, s.content.organization)
	if err != nil {
		return nil, fmt.Errorf("error on find devices in repository: %w", err)
	}
	if len(list) == 0 {
		if _, _, err := s.content.open(ctx, rawProjectID); err != nil {
			return nil, err
		}
	}
	return appdto.NewDeviceList(list), nil
}

func (s deviceServiceImpl) FindDeviceByID(ctx context.Context, rawProjectID, rawDeviceID uuid.UUID) (appdto.Device, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Device{}, err
	}
	found, err := scope.Devices().FindByID(ctx, id.NewID(id.IDWithUUID[device.Device](rawDeviceID)))
	if err != nil {
		return appdto.Device{}, fmt.Errorf("error on find device by id in repository: %w", err)
	}
	return appdto.NewDevice(found), nil
}

func (s deviceServiceImpl) FindDeviceByName(ctx context.Context, rawProjectID uuid.UUID, rawName string) (appdto.Device, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Device{}, err
	}
	name, err := device.NewDeviceName(rawName)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot find device because of name: %w", invalidInput(err))
	}
	found, err := scope.Devices().FindByName(ctx, name)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("error on find device by name in repository: %w", err)
	}
	return appdto.NewDevice(found), nil
}

func (s deviceServiceImpl) CreateDevice(ctx context.Context, rawProjectID uuid.UUID, input appdto.DeviceInput) (appdto.Device, error) {
	projectID, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Device{}, err
	}
	fields, err := parseDeviceInput(input)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot create device: %w", err)
	}
	repo := scope.Devices()
	created, err := device.CreateDevice(repo.NextID(), projectID, fields.name, fields.comment, fields.protocol, fields.config)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot create device: %w", err)
	}
	saved, err := repo.Save(ctx, created)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot save device: %w", err)
	}
	return appdto.NewDevice(saved), nil
}

func (s deviceServiceImpl) UpdateDevice(ctx context.Context, rawProjectID, rawDeviceID uuid.UUID, rawExpectedVersion int, input appdto.DeviceInput) (appdto.Device, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Device{}, err
	}
	// The stated Version is the caller's input: a negative one is no version at
	// all, answered before the Device is read.
	expected, err := deviceVersion(rawExpectedVersion)
	if err != nil {
		return appdto.Device{}, err
	}
	repo := scope.Devices()
	found, err := repo.FindByID(ctx, id.NewID(id.IDWithUUID[device.Device](rawDeviceID)))
	if err != nil {
		return appdto.Device{}, fmt.Errorf("error on find device by id in repository: %w", err)
	}
	// A stale write is a conflict whatever it sends.
	if err := found.CheckVersion(expected); err != nil {
		return appdto.Device{}, fmt.Errorf("cannot update device: %w", err)
	}
	fields, err := parseDeviceInput(input)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot update device: %w", err)
	}
	updated, err := found.Update(expected, fields.name, fields.comment, fields.protocol, fields.config)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot update device: %w", err)
	}
	saved, err := repo.Save(ctx, updated)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot save device: %w", err)
	}
	return appdto.NewDevice(saved), nil
}

// DeleteDeviceByID removes a Device and returns it as it was read. It expects
// no Version (retryOnRace): removing does not rest on what a client saw.
func (s deviceServiceImpl) DeleteDeviceByID(ctx context.Context, rawProjectID, rawDeviceID uuid.UUID) (appdto.Device, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Device{}, err
	}
	repo := scope.Devices()
	deviceID := id.NewID(id.IDWithUUID[device.Device](rawDeviceID))
	var deleted device.Device
	err = retryOnRace(device.ErrDeviceConflict, func() error {
		found, err := repo.FindByID(ctx, deviceID)
		if err != nil {
			return err
		}
		deleted = found
		return repo.Delete(ctx, found.Delete())
	})
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot delete device: %w", err)
	}
	return appdto.NewDevice(deleted), nil
}

// deviceFields are the parsed fields of a DeviceInput.
type deviceFields struct {
	name     device.DeviceName
	comment  string
	protocol device.DeviceProtocol
	config   device.DeviceConfig
}

// parseDeviceInput parses the fields of a DeviceInput; an error marks invalid
// input.
func parseDeviceInput(input appdto.DeviceInput) (deviceFields, error) {
	name, err := device.NewDeviceName(input.Name)
	if err != nil {
		return deviceFields{}, invalidInput(fmt.Errorf("name: %w", err))
	}
	protocol, err := device.NewDeviceProtocol(input.Protocol)
	if err != nil {
		return deviceFields{}, invalidInput(fmt.Errorf("protocol: %w", err))
	}
	config, err := device.NewDeviceConfig(input.Config)
	if err != nil {
		return deviceFields{}, invalidInput(fmt.Errorf("config: %w", err))
	}
	return deviceFields{name: name, comment: input.Comment, protocol: protocol, config: config}, nil
}

// deviceVersion parses the Version a client states; a negative one is no
// version at all: the client's input, not a conflict.
func deviceVersion(raw int) (version.Version[device.Device], error) {
	expected, err := version.New(version.WithNumber[device.Device](raw))
	if err != nil {
		return expected, invalidInput(fmt.Errorf("version: %w", err))
	}
	return expected, nil
}

// environmentVersion parses the Version of an Environment a client states.
func environmentVersion(raw int) (version.Version[device.Environment], error) {
	expected, err := version.New(version.WithNumber[device.Environment](raw))
	if err != nil {
		return expected, invalidInput(fmt.Errorf("version: %w", err))
	}
	return expected, nil
}

// environmentDeviceVersion parses the Version of a pair a client states.
func environmentDeviceVersion(raw int) (version.Version[device.EnvironmentDevice], error) {
	expected, err := version.New(version.WithNumber[device.EnvironmentDevice](raw))
	if err != nil {
		return expected, invalidInput(fmt.Errorf("version: %w", err))
	}
	return expected, nil
}

// pairOfProject finds the pair of the Environment and the Device, both of the
// Project.
func pairOfProject(ctx context.Context, repo device.EnvironmentDeviceRepository, environmentID id.ID[device.Environment], deviceID id.ID[device.Device]) (device.EnvironmentDevice, error) {
	found, err := repo.FindByPair(ctx, environmentID, deviceID)
	if errors.Is(err, device.ErrEnvironmentDeviceNotFound) {
		return nil, fmt.Errorf("%w: environment %s, device %s", device.ErrEnvironmentDeviceNotFound, environmentID, deviceID)
	}
	if err != nil {
		return nil, fmt.Errorf("error on find environment device in repository: %w", err)
	}
	return found, nil
}
