package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// cleanDevices removes the Tags, the pairs, the Devices and the Environments
// of the test database: the pairs first, then the rows they refer to.
func cleanDevices(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`DELETE FROM runtime.deployments; DELETE FROM runtime.tags; DELETE FROM engineering.tags;
		 DELETE FROM engineering.environment_devices; DELETE FROM engineering.devices; DELETE FROM engineering.environments;
		 UPDATE engineering.drafts SET clean_version = 0`); err != nil {
		t.Fatalf("cleanDevices: %v", err)
	}
}

func newDeviceService() application.DeviceService {
	return application.NewDeviceService(contentOver(newScopes()))
}

func newEnvironmentService() application.EnvironmentService {
	return application.NewEnvironmentService(contentOver(newScopes()))
}

// newDevices creates a Device of the Project Main and returns the service and
// the Device.
func newDevices(t *testing.T) (application.DeviceService, appdto.Device) {
	t.Helper()
	svc := newDeviceService()
	created, err := svc.CreateDevice(context.Background(), mainProject, draftVersionOf(t, mainProject), appdto.DeviceInput{
		Name: "device-" + uuid.NewString(), Comment: "the pump", Protocol: "simulation", Config: []byte(`{"pattern":"ramp"}`),
	})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	return svc, created
}

func TestDeviceService_CRUD(t *testing.T) {
	cleanDevices(t)
	ctx := context.Background()
	svc, created := newDevices(t)

	if created.Version == 0 || created.Protocol != "simulation" {
		t.Errorf("created %+v", created)
	}
	if all, err := svc.FindAllDevices(ctx, mainProject); err != nil || len(all) != 1 || all[0].ID != created.ID {
		t.Errorf("FindAllDevices: %v, %v", all, err)
	}
	if found, err := svc.FindDeviceByID(ctx, mainProject, created.ID); err != nil || found.Name != created.Name {
		t.Errorf("FindDeviceByID: %+v, %v", found, err)
	}
	if found, err := svc.FindDeviceByName(ctx, mainProject, created.Name); err != nil || found.ID != created.ID {
		t.Errorf("FindDeviceByName: %+v, %v", found, err)
	}

	updated, err := svc.UpdateDevice(ctx, mainProject, created.ID, created.Version, appdto.DeviceInput{
		Name: created.Name, Comment: "renamed", Protocol: "simulation", Config: []byte(`{"pattern":"sine"}`),
	})
	if err != nil || updated.Version != created.Version+1 || updated.Comment != "renamed" {
		t.Fatalf("UpdateDevice: %+v, %v", updated, err)
	}
	if _, err := svc.UpdateDevice(ctx, mainProject, created.ID, created.Version, appdto.DeviceInput{Name: created.Name, Protocol: "simulation"}); !errors.Is(err, draft.ErrDraftConflict) {
		t.Errorf("a stale update: %v, want draft.ErrDraftConflict", err)
	}
	if _, err := svc.CreateDevice(ctx, mainProject, draftVersionOf(t, mainProject), appdto.DeviceInput{Name: created.Name, Protocol: "simulation"}); !errors.Is(err, device.ErrDeviceNameTaken) {
		t.Errorf("a taken name: %v, want ErrDeviceNameTaken", err)
	}
	if _, err := svc.CreateDevice(ctx, mainProject, draftVersionOf(t, mainProject), appdto.DeviceInput{Name: "bad-protocol-" + uuid.NewString(), Protocol: "modbus"}); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("an unknown protocol: %v, want ErrInvalidInput", err)
	}

	if _, err := svc.DeleteDeviceByID(ctx, mainProject, created.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteDeviceByID: %v", err)
	}
	if _, err := svc.FindDeviceByID(ctx, mainProject, created.ID); !errors.Is(err, device.ErrDeviceNotFound) {
		t.Errorf("after Delete: %v, want ErrDeviceNotFound", err)
	}
}

func TestEnvironmentService_CRUDAndPairs(t *testing.T) {
	cleanDevices(t)
	ctx := context.Background()
	devices, created := newDevices(t)
	environments := newEnvironmentService()

	environment, err := environments.CreateEnvironment(ctx, mainProject, draftVersionOf(t, mainProject), appdto.EnvironmentInput{Name: "bench-" + uuid.NewString(), Comment: "the bench"})
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if all, err := environments.FindAllEnvironments(ctx, mainProject); err != nil || len(all) != 1 || all[0].ID != environment.ID {
		t.Errorf("FindAllEnvironments: %v, %v", all, err)
	}
	renamed, err := environments.UpdateEnvironment(ctx, mainProject, environment.ID, environment.Version, appdto.EnvironmentInput{Name: environment.Name, Comment: "renamed"})
	if err != nil || renamed.Version != environment.Version+1 {
		t.Fatalf("UpdateEnvironment: %+v, %v", renamed, err)
	}

	pair, added, err := environments.PutEnvironmentDevice(ctx, mainProject, environment.ID, created.ID, draftVersionOf(t, mainProject), appdto.EnvironmentDeviceInput{Params: []byte(`{"host":"localhost"}`)})
	if err != nil || !added || pair.Version == 0 {
		t.Fatalf("PutEnvironmentDevice add: %+v, %v, %v", pair, added, err)
	}
	if list, err := environments.FindEnvironmentDevices(ctx, mainProject, environment.ID); err != nil || len(list) != 1 || list[0].DeviceID != created.ID {
		t.Errorf("FindEnvironmentDevices: %v, %v", list, err)
	}
	changed, added, err := environments.PutEnvironmentDevice(ctx, mainProject, environment.ID, created.ID, pair.Version, appdto.EnvironmentDeviceInput{Params: []byte(`{"host":"10.0.0.1"}`)})
	if err != nil || added || changed.Version != pair.Version+1 {
		t.Fatalf("PutEnvironmentDevice change: %+v, %v, %v", changed, added, err)
	}
	if _, _, err := environments.PutEnvironmentDevice(ctx, mainProject, environment.ID, created.ID, pair.Version, appdto.EnvironmentDeviceInput{}); !errors.Is(err, draft.ErrDraftConflict) {
		t.Errorf("a stale pair write: %v, want draft.ErrDraftConflict", err)
	}
	// A stale write is a conflict whatever it sends, as for a Device or an
	// Environment; the params are parsed only once the Version is current.
	if _, _, err := environments.PutEnvironmentDevice(ctx, mainProject, environment.ID, created.ID, pair.Version, appdto.EnvironmentDeviceInput{Params: []byte(`[]`)}); !errors.Is(err, draft.ErrDraftConflict) {
		t.Errorf("a stale pair write with invalid params: %v, want draft.ErrDraftConflict", err)
	}
	if _, _, err := environments.PutEnvironmentDevice(ctx, mainProject, environment.ID, created.ID, changed.Version, appdto.EnvironmentDeviceInput{Params: []byte(`[]`)}); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("a current pair write with invalid params: %v, want ErrInvalidInput", err)
	}

	// A pair refuses removing its Environment and its Device; without one both go.
	if _, err := environments.DeleteEnvironmentByID(ctx, mainProject, environment.ID, draftVersionOf(t, mainProject)); !errors.Is(err, device.ErrEnvironmentInUse) {
		t.Errorf("an environment holding a pair: %v, want ErrEnvironmentInUse", err)
	}
	if _, err := devices.DeleteDeviceByID(ctx, mainProject, created.ID, draftVersionOf(t, mainProject)); !errors.Is(err, device.ErrDeviceInUse) {
		t.Errorf("a device in a pair: %v, want ErrDeviceInUse", err)
	}
	if _, err := environments.DeleteEnvironmentDevice(ctx, mainProject, environment.ID, created.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteEnvironmentDevice: %v", err)
	}
	if _, err := environments.DeleteEnvironmentByID(ctx, mainProject, environment.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Errorf("DeleteEnvironmentByID after the pair: %v", err)
	}
	if _, err := devices.DeleteDeviceByID(ctx, mainProject, created.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Errorf("DeleteDeviceByID after the pair: %v", err)
	}
}

func TestDeviceService_ProjectScope(t *testing.T) {
	cleanProjects(t)
	cleanDevices(t)
	ctx := context.Background()
	projects := application.NewProjectService(repositories.NewProjectRepositoryPostgres(testDB), repositories.NewDraftRepositoryPostgres(testDB, repositories.NewLibraryReleaseRepositoryPostgres(testDB)), soleOrganization())
	other, err := projects.CreateProject(ctx, appdto.ProjectInput{Name: "Devices Other " + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	_, mine := newDevices(t)
	if _, err := newDeviceService().FindDeviceByID(ctx, other.ID, mine.ID); !errors.Is(err, device.ErrDeviceNotFound) {
		t.Errorf("a device of Main through another project: %v, want ErrDeviceNotFound", err)
	}
	if _, err := newDeviceService().FindDeviceByID(ctx, uuid.New(), mine.ID); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("a missing project: %v, want ErrProjectNotFound", err)
	}

	// The lists are read with the check of their Project in one query, as the
	// Tags and the Scenes are; an empty list still tells a missing Project
	// from an empty one.
	otherDevice, err := newDeviceService().CreateDevice(ctx, other.ID, draftVersionOf(t, other.ID), appdto.DeviceInput{Name: "other-" + uuid.NewString(), Protocol: "simulation"})
	if err != nil {
		t.Fatalf("CreateDevice in the other project: %v", err)
	}
	if _, err := newEnvironmentService().CreateEnvironment(ctx, other.ID, draftVersionOf(t, other.ID), appdto.EnvironmentInput{Name: "other-" + uuid.NewString()}); err != nil {
		t.Fatalf("CreateEnvironment in the other project: %v", err)
	}
	if list, err := newDeviceService().FindAllDevices(ctx, other.ID); err != nil || len(list) != 1 || list[0].ID != otherDevice.ID {
		t.Errorf("FindAllDevices of the other project: %v, %v", list, err)
	}
	if list, err := newEnvironmentService().FindAllEnvironments(ctx, other.ID); err != nil || len(list) != 1 {
		t.Errorf("FindAllEnvironments of the other project: %v, %v", list, err)
	}
	if _, err := newDeviceService().FindAllDevices(ctx, uuid.New()); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("FindAllDevices of a missing project: %v, want ErrProjectNotFound", err)
	}
	if _, err := newEnvironmentService().FindAllEnvironments(ctx, uuid.New()); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("FindAllEnvironments of a missing project: %v, want ErrProjectNotFound", err)
	}

	// A marked Project has no lists, which is how the UI leaves it at once.
	if _, err := projects.DeleteProjectByID(ctx, other.ID); err != nil {
		t.Fatalf("DeleteProjectByID: %v", err)
	}
	if list, err := newDeviceService().FindAllDevices(ctx, other.ID); !errors.Is(err, project.ErrProjectNotFound) || list != nil {
		t.Errorf("FindAllDevices of a marked project: %v, %v; want ErrProjectNotFound", list, err)
	}
	if list, err := newEnvironmentService().FindAllEnvironments(ctx, other.ID); !errors.Is(err, project.ErrProjectNotFound) || list != nil {
		t.Errorf("FindAllEnvironments of a marked project: %v, %v; want ErrProjectNotFound", list, err)
	}
}
