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

// EnvironmentService is the service interface for working with the
// Environments of a Project's Draft and the parameters of its Devices in
// them (ADR 0004, amendment 2026-11-01). An Environment is a named set of the
// Project's Devices as they stand in one context of operation; a pair
// (Environment, Device) carries the parameters by which the Device is reached
// there, and is identified by the two, with no ID of its own. Every write
// states the Version of the Draft (ADR 0016). Every method takes the project
// first; a project that does not exist (or is deleted) is an error wrapping
// project.ErrProjectNotFound.
type EnvironmentService interface {
	FindAllEnvironments(ctx context.Context, projectID uuid.UUID) ([]appdto.Environment, error)
	FindEnvironmentByID(ctx context.Context, projectID, environmentID uuid.UUID) (appdto.Environment, error)
	FindEnvironmentByName(ctx context.Context, projectID uuid.UUID, name string) (appdto.Environment, error)
	CreateEnvironment(ctx context.Context, projectID uuid.UUID, version int, input appdto.EnvironmentInput) (appdto.Environment, error)
	// UpdateEnvironment changes an Environment, expecting the Version of the
	// Draft.
	UpdateEnvironment(ctx context.Context, projectID, environmentID uuid.UUID, version int, input appdto.EnvironmentInput) (appdto.Environment, error)
	// DeleteEnvironmentByID removes an Environment, expecting the Version of
	// the Draft; one that still holds the pairs of its Devices is not removed
	// (device.ErrEnvironmentInUse).
	DeleteEnvironmentByID(ctx context.Context, projectID, environmentID uuid.UUID, version int) (appdto.Environment, error)

	// FindEnvironmentDevices returns the pairs of the Environment.
	FindEnvironmentDevices(ctx context.Context, projectID, environmentID uuid.UUID) ([]appdto.EnvironmentDevice, error)
	// PutEnvironmentDevice sets the parameters of the pair: it adds the pair
	// when the Environment does not hold the Device, else changes it, stating
	// the Version of the Draft. The bool reports whether the pair was added.
	PutEnvironmentDevice(ctx context.Context, projectID, environmentID, deviceID uuid.UUID, version int, input appdto.EnvironmentDeviceInput) (appdto.EnvironmentDevice, bool, error)
	// DeleteEnvironmentDevice removes the pair, expecting the Version of the
	// Draft.
	DeleteEnvironmentDevice(ctx context.Context, projectID, environmentID, deviceID uuid.UUID, version int) (appdto.EnvironmentDevice, error)
}

type environmentServiceImpl struct {
	content ProjectContent
}

var _ EnvironmentService = (*environmentServiceImpl)(nil)

// NewEnvironmentService returns the service of the Environments and their
// pairs. The events of its changes are recorded by the entities and saved with
// the Draft (ADR 0016).
func NewEnvironmentService(aContent ProjectContent) EnvironmentService {
	return &environmentServiceImpl{content: aContent}
}

// FindAllEnvironments returns all Environments of the Project. The UI
// reloads the list on every event, so the Environments are read with the
// check of their Project in one query, and the Project is read apart only
// when there are none, to tell a missing Project from an empty one.
func (s environmentServiceImpl) FindAllEnvironments(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.Environment, error) {
	_, scope := s.content.openUnchecked(rawProjectID)
	list, err := scope.Environments().FindAllInLiveProject(ctx, s.content.organization)
	if err != nil {
		return nil, fmt.Errorf("error on find environments in repository: %w", err)
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
	return appdto.NewEnvironmentList(list, version), nil
}

func (s environmentServiceImpl) FindEnvironmentByID(ctx context.Context, rawProjectID, rawEnvironmentID uuid.UUID) (appdto.Environment, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Environment{}, err
	}
	found, err := scope.Environments().FindByID(ctx, id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID)))
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("error on find environment by id in repository: %w", err)
	}
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return appdto.Environment{}, err
	}
	return appdto.NewEnvironment(found, version), nil
}

func (s environmentServiceImpl) FindEnvironmentByName(ctx context.Context, rawProjectID uuid.UUID, rawName string) (appdto.Environment, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Environment{}, err
	}
	name, err := device.NewEnvironmentName(rawName)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot find environment because of name: %w", invalidInput(err))
	}
	found, err := scope.Environments().FindByName(ctx, name)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("error on find environment by name in repository: %w", err)
	}
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return appdto.Environment{}, err
	}
	return appdto.NewEnvironment(found, version), nil
}

func (s environmentServiceImpl) CreateEnvironment(ctx context.Context, rawProjectID uuid.UUID, rawVersion int, input appdto.EnvironmentInput) (appdto.Environment, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Environment{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Environment{}, err
	}
	name, err := device.NewEnvironmentName(input.Name)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot create environment: name: %w", invalidInput(err))
	}
	created, err := device.CreateEnvironment(id.NewID[device.Environment](), projectID, name, input.Comment)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot create environment: %w", err)
	}
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Create environment "+name.String(),
		func(session *draft.Session) error {
			session.CreateEnvironment(ctx, created)
			return nil
		})
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot save environment: %w", err)
	}
	return appdto.NewEnvironment(created, result.Version().Number()), nil
}

func (s environmentServiceImpl) UpdateEnvironment(ctx context.Context, rawProjectID, rawEnvironmentID uuid.UUID, rawVersion int, input appdto.EnvironmentInput) (appdto.Environment, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Environment{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Environment{}, err
	}
	name, err := device.NewEnvironmentName(input.Name)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot update environment: name: %w", invalidInput(err))
	}
	environmentID := id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID))
	var updated device.Environment
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Update environment "+name.String(),
		func(session *draft.Session) error {
			var err error
			updated, err = session.UpdateEnvironment(ctx, environmentID, name, input.Comment)
			return err
		})
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot update environment: %w", err)
	}
	return appdto.NewEnvironment(updated, result.Version().Number()), nil
}

// DeleteEnvironmentByID removes an Environment and returns it as it was read.
func (s environmentServiceImpl) DeleteEnvironmentByID(ctx context.Context, rawProjectID, rawEnvironmentID uuid.UUID, rawVersion int) (appdto.Environment, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Environment{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Environment{}, err
	}
	environmentID := id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID))
	var deleted device.Environment
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Delete environment",
		func(session *draft.Session) error {
			var err error
			deleted, err = session.DeleteEnvironment(ctx, environmentID)
			return err
		})
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot delete environment: %w", err)
	}
	return appdto.NewEnvironment(deleted, result.Version().Number()), nil
}

func (s environmentServiceImpl) FindEnvironmentDevices(ctx context.Context, rawProjectID, rawEnvironmentID uuid.UUID) ([]appdto.EnvironmentDevice, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	list, err := scope.EnvironmentDevices().FindByEnvironment(ctx, id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID)))
	if err != nil {
		return nil, fmt.Errorf("error on find environment devices in repository: %w", err)
	}
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	return appdto.NewEnvironmentDeviceList(list, version), nil
}

func (s environmentServiceImpl) PutEnvironmentDevice(ctx context.Context, rawProjectID, rawEnvironmentID, rawDeviceID uuid.UUID, rawVersion int, input appdto.EnvironmentDeviceInput) (appdto.EnvironmentDevice, bool, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.EnvironmentDevice{}, false, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.EnvironmentDevice{}, false, err
	}
	environmentID := id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID))
	deviceID := id.NewID(id.IDWithUUID[device.Device](rawDeviceID))
	var saved device.EnvironmentDevice
	added := false
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Set environment device",
		func(session *draft.Session) error {
			params, err := device.NewEnvironmentDeviceParams(input.Params)
			if err != nil {
				return invalidInput(fmt.Errorf("params: %w", err))
			}
			saved, added, err = session.PutEnvironmentDevice(ctx, environmentID, deviceID, params)
			return err
		})
	if err != nil {
		return appdto.EnvironmentDevice{}, false, fmt.Errorf("cannot save environment device: %w", err)
	}
	return appdto.NewEnvironmentDevice(saved, result.Version().Number()), added, nil
}

func (s environmentServiceImpl) DeleteEnvironmentDevice(ctx context.Context, rawProjectID, rawEnvironmentID, rawDeviceID uuid.UUID, rawVersion int) (appdto.EnvironmentDevice, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.EnvironmentDevice{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.EnvironmentDevice{}, err
	}
	environmentID := id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID))
	deviceID := id.NewID(id.IDWithUUID[device.Device](rawDeviceID))
	var deleted device.EnvironmentDevice
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Delete environment device",
		func(session *draft.Session) error {
			var err error
			deleted, err = session.DeleteEnvironmentDevice(ctx, environmentID, deviceID)
			return err
		})
	if err != nil {
		return appdto.EnvironmentDevice{}, fmt.Errorf("cannot delete environment device: %w", err)
	}
	return appdto.NewEnvironmentDevice(deleted, result.Version().Number()), nil
}
