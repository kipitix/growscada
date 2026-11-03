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

// EnvironmentService is the service interface for working with the
// Environments of a Project's Draft and the parameters of its Devices in
// them (ADR 0004, amendment 2026-11-01). An Environment is a named set of the
// Project's Devices as they stand in one context of operation; a pair
// (Environment, Device) carries the parameters by which the Device is reached
// there, and is identified by the two, with no ID of its own. Every method
// takes the project first; a project that does not exist (or is deleted) is
// an error wrapping project.ErrProjectNotFound.
type EnvironmentService interface {
	FindAllEnvironments(ctx context.Context, projectID uuid.UUID) ([]appdto.Environment, error)
	FindEnvironmentByID(ctx context.Context, projectID, environmentID uuid.UUID) (appdto.Environment, error)
	FindEnvironmentByName(ctx context.Context, projectID uuid.UUID, name string) (appdto.Environment, error)
	CreateEnvironment(ctx context.Context, projectID uuid.UUID, input appdto.EnvironmentInput) (appdto.Environment, error)
	// UpdateEnvironment changes an Environment, expecting its Version.
	UpdateEnvironment(ctx context.Context, projectID, environmentID uuid.UUID, version int, input appdto.EnvironmentInput) (appdto.Environment, error)
	// DeleteEnvironmentByID removes an Environment, expecting no Version; one
	// that still holds the pairs of its Devices is not removed
	// (device.ErrEnvironmentInUse).
	DeleteEnvironmentByID(ctx context.Context, projectID, environmentID uuid.UUID) (appdto.Environment, error)

	// FindEnvironmentDevices returns the pairs of the Environment.
	FindEnvironmentDevices(ctx context.Context, projectID, environmentID uuid.UUID) ([]appdto.EnvironmentDevice, error)
	// PutEnvironmentDevice sets the parameters of the pair: it adds the pair
	// when version is 0, else changes it, expecting its Version. The bool
	// reports whether the pair was added.
	PutEnvironmentDevice(ctx context.Context, projectID, environmentID, deviceID uuid.UUID, version int, input appdto.EnvironmentDeviceInput) (appdto.EnvironmentDevice, bool, error)
	// DeleteEnvironmentDevice removes the pair, expecting no Version.
	DeleteEnvironmentDevice(ctx context.Context, projectID, environmentID, deviceID uuid.UUID) (appdto.EnvironmentDevice, error)
}

type environmentServiceImpl struct {
	content ProjectContent
}

var _ EnvironmentService = (*environmentServiceImpl)(nil)

// NewEnvironmentService returns the service of the Environments and their
// pairs. The events of its changes are recorded by the aggregates and saved
// with them (ADR 0008).
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
	return appdto.NewEnvironmentList(list), nil
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
	return appdto.NewEnvironment(found), nil
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
	return appdto.NewEnvironment(found), nil
}

func (s environmentServiceImpl) CreateEnvironment(ctx context.Context, rawProjectID uuid.UUID, input appdto.EnvironmentInput) (appdto.Environment, error) {
	projectID, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Environment{}, err
	}
	name, err := device.NewEnvironmentName(input.Name)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot create environment: name: %w", invalidInput(err))
	}
	repo := scope.Environments()
	created, err := device.CreateEnvironment(repo.NextID(), projectID, name, input.Comment)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot create environment: %w", err)
	}
	saved, err := repo.Save(ctx, created)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot save environment: %w", err)
	}
	return appdto.NewEnvironment(saved), nil
}

func (s environmentServiceImpl) UpdateEnvironment(ctx context.Context, rawProjectID, rawEnvironmentID uuid.UUID, rawExpectedVersion int, input appdto.EnvironmentInput) (appdto.Environment, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Environment{}, err
	}
	// The stated Version is the caller's input: a negative one is no version at
	// all, answered before the Environment is read.
	expected, err := environmentVersion(rawExpectedVersion)
	if err != nil {
		return appdto.Environment{}, err
	}
	repo := scope.Environments()
	found, err := repo.FindByID(ctx, id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID)))
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("error on find environment by id in repository: %w", err)
	}
	// A stale write is a conflict whatever it sends.
	if err := found.CheckVersion(expected); err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot update environment: %w", err)
	}
	name, err := device.NewEnvironmentName(input.Name)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot update environment: name: %w", invalidInput(err))
	}
	updated, err := found.Update(expected, name, input.Comment)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot update environment: %w", err)
	}
	saved, err := repo.Save(ctx, updated)
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot save environment: %w", err)
	}
	return appdto.NewEnvironment(saved), nil
}

// DeleteEnvironmentByID removes an Environment and returns it as it was read.
// It expects no Version (retryOnRace): removing does not rest on what a client
// saw.
func (s environmentServiceImpl) DeleteEnvironmentByID(ctx context.Context, rawProjectID, rawEnvironmentID uuid.UUID) (appdto.Environment, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Environment{}, err
	}
	repo := scope.Environments()
	environmentID := id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID))
	var deleted device.Environment
	err = retryOnRace(device.ErrEnvironmentConflict, func() error {
		found, err := repo.FindByID(ctx, environmentID)
		if err != nil {
			return err
		}
		deleted = found
		return repo.Delete(ctx, found.Delete())
	})
	if err != nil {
		return appdto.Environment{}, fmt.Errorf("cannot delete environment: %w", err)
	}
	return appdto.NewEnvironment(deleted), nil
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
	return appdto.NewEnvironmentDeviceList(list), nil
}

func (s environmentServiceImpl) PutEnvironmentDevice(ctx context.Context, rawProjectID, rawEnvironmentID, rawDeviceID uuid.UUID, rawExpectedVersion int, input appdto.EnvironmentDeviceInput) (appdto.EnvironmentDevice, bool, error) {
	projectID, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.EnvironmentDevice{}, false, err
	}
	environmentID := id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID))
	deviceID := id.NewID(id.IDWithUUID[device.Device](rawDeviceID))
	// A negative version is no version at all: the client's input, not a conflict.
	expected, err := environmentDeviceVersion(rawExpectedVersion)
	if err != nil {
		return appdto.EnvironmentDevice{}, false, err
	}
	repo := scope.EnvironmentDevices()
	found, err := repo.FindByPair(ctx, environmentID, deviceID)
	switch {
	case errors.Is(err, device.ErrEnvironmentDeviceNotFound) && expected == version.Initial[device.EnvironmentDevice]():
		params, err := device.NewEnvironmentDeviceParams(input.Params)
		if err != nil {
			return appdto.EnvironmentDevice{}, false, invalidInput(fmt.Errorf("params: %w", err))
		}
		created, err := repo.Save(ctx, device.CreateEnvironmentDevice(projectID, environmentID, deviceID, params))
		if err != nil {
			return appdto.EnvironmentDevice{}, false, fmt.Errorf("cannot save environment device: %w", err)
		}
		return appdto.NewEnvironmentDevice(created), true, nil
	case err != nil:
		return appdto.EnvironmentDevice{}, false, fmt.Errorf("error on find environment device in repository: %w", err)
	}

	// A stale write is a conflict whatever it sends.
	if err := found.CheckVersion(expected); err != nil {
		return appdto.EnvironmentDevice{}, false, fmt.Errorf("cannot update environment device: %w", err)
	}
	params, err := device.NewEnvironmentDeviceParams(input.Params)
	if err != nil {
		return appdto.EnvironmentDevice{}, false, invalidInput(fmt.Errorf("params: %w", err))
	}
	updated, err := found.UpdateParams(expected, params)
	if err != nil {
		return appdto.EnvironmentDevice{}, false, fmt.Errorf("cannot update environment device: %w", err)
	}
	saved, err := repo.Save(ctx, updated)
	if err != nil {
		return appdto.EnvironmentDevice{}, false, fmt.Errorf("cannot save environment device: %w", err)
	}
	return appdto.NewEnvironmentDevice(saved), false, nil
}

func (s environmentServiceImpl) DeleteEnvironmentDevice(ctx context.Context, rawProjectID, rawEnvironmentID, rawDeviceID uuid.UUID) (appdto.EnvironmentDevice, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.EnvironmentDevice{}, err
	}
	repo := scope.EnvironmentDevices()
	environmentID := id.NewID(id.IDWithUUID[device.Environment](rawEnvironmentID))
	deviceID := id.NewID(id.IDWithUUID[device.Device](rawDeviceID))
	var deleted device.EnvironmentDevice
	err = retryOnRace(device.ErrEnvironmentDeviceConflict, func() error {
		found, err := pairOfProject(ctx, repo, environmentID, deviceID)
		if err != nil {
			return err
		}
		deleted = found
		return repo.Delete(ctx, found.Delete())
	})
	if err != nil {
		return appdto.EnvironmentDevice{}, fmt.Errorf("cannot delete environment device: %w", err)
	}
	return appdto.NewEnvironmentDevice(deleted), nil
}
