package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// DeviceService is the service interface for working with the Devices of a
// Project's Draft: what its Tags may be supplied by (ADR 0004, amendment
// 2026-11-01). The parameters by which a Device is reached in an Environment
// are the EnvironmentService's. Every write states the Version of the Draft
// (ADR 0016). Every method takes the project first; a project that does not
// exist (or is deleted) is an error wrapping project.ErrProjectNotFound.
type DeviceService interface {
	FindAllDevices(ctx context.Context, projectID uuid.UUID) ([]appdto.Device, error)
	FindDeviceByID(ctx context.Context, projectID, deviceID uuid.UUID) (appdto.Device, error)
	FindDeviceByName(ctx context.Context, projectID uuid.UUID, name string) (appdto.Device, error)
	CreateDevice(ctx context.Context, projectID uuid.UUID, version int, input appdto.DeviceInput) (appdto.Device, error)
	// UpdateDevice changes a Device, expecting the Version of the Draft.
	UpdateDevice(ctx context.Context, projectID, deviceID uuid.UUID, version int, input appdto.DeviceInput) (appdto.Device, error)
	// DeleteDeviceByID removes a Device, expecting the Version of the Draft;
	// a Device a Tag is supplied by, or one that stands in an Environment, is
	// not removed (device.ErrDeviceInUse).
	DeleteDeviceByID(ctx context.Context, projectID, deviceID uuid.UUID, version int) (appdto.Device, error)
}

type deviceServiceImpl struct {
	content ProjectContent
}

var _ DeviceService = (*deviceServiceImpl)(nil)

// NewDeviceService returns the service of the Devices. The events of its
// changes are recorded by the Device and saved with the Draft (ADR 0016).
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
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	return appdto.NewDeviceList(list, version), nil
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
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return appdto.Device{}, err
	}
	return appdto.NewDevice(found, version), nil
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
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return appdto.Device{}, err
	}
	return appdto.NewDevice(found, version), nil
}

func (s deviceServiceImpl) CreateDevice(ctx context.Context, rawProjectID uuid.UUID, rawVersion int, input appdto.DeviceInput) (appdto.Device, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Device{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Device{}, err
	}
	fields, err := parseDeviceInput(input)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot create device: %w", err)
	}
	created, err := device.CreateDevice(id.NewID[device.Device](), projectID, fields.name, fields.comment, fields.protocol, fields.config)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot create device: %w", err)
	}
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Create device "+fields.name.String(),
		func(session *draft.Session) error {
			session.CreateDevice(ctx, created)
			return nil
		})
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot save device: %w", err)
	}
	return appdto.NewDevice(created, result.Version().Number()), nil
}

func (s deviceServiceImpl) UpdateDevice(ctx context.Context, rawProjectID, rawDeviceID uuid.UUID, rawVersion int, input appdto.DeviceInput) (appdto.Device, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Device{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Device{}, err
	}
	fields, err := parseDeviceInput(input)
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot update device: %w", err)
	}
	deviceID := id.NewID(id.IDWithUUID[device.Device](rawDeviceID))
	var updated device.Device
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Update device "+fields.name.String(),
		func(session *draft.Session) error {
			var err error
			updated, err = session.UpdateDevice(ctx, deviceID, fields.name, fields.comment, fields.protocol, fields.config)
			return err
		})
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot update device: %w", err)
	}
	return appdto.NewDevice(updated, result.Version().Number()), nil
}

// DeleteDeviceByID removes a Device and returns it as it was read, stating
// the Version of the Draft.
func (s deviceServiceImpl) DeleteDeviceByID(ctx context.Context, rawProjectID, rawDeviceID uuid.UUID, rawVersion int) (appdto.Device, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Device{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Device{}, err
	}
	deviceID := id.NewID(id.IDWithUUID[device.Device](rawDeviceID))
	var deleted device.Device
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Delete device",
		func(session *draft.Session) error {
			var err error
			deleted, err = session.DeleteDevice(ctx, deviceID)
			return err
		})
	if err != nil {
		return appdto.Device{}, fmt.Errorf("cannot delete device: %w", err)
	}
	return appdto.NewDevice(deleted, result.Version().Number()), nil
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
