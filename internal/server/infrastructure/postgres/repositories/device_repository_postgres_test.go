package repositories_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// cleanDevices removes the Tags, the pairs, the Devices and the Environments
// of every Project: the pairs first, then the rows they refer to.
func cleanDevices(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`DELETE FROM runtime.tags; DELETE FROM engineering.tags;
		 DELETE FROM engineering.environment_devices; DELETE FROM engineering.devices; DELETE FROM engineering.environments`); err != nil {
		t.Fatalf("cleanDevices: %v", err)
	}
}

func mustDeviceName(t *testing.T, name string) device.DeviceName {
	t.Helper()
	parsed, err := device.NewDeviceName(name)
	if err != nil {
		t.Fatalf("NewDeviceName(%q): %v", name, err)
	}
	return parsed
}

func mustEnvironmentName(t *testing.T, name string) device.EnvironmentName {
	t.Helper()
	parsed, err := device.NewEnvironmentName(name)
	if err != nil {
		t.Fatalf("NewEnvironmentName(%q): %v", name, err)
	}
	return parsed
}

// mustSaveDevice stores a new Device of the project, of the simulation
// protocol and an empty configuration.
func mustSaveDevice(t *testing.T, repo device.DeviceRepository, projectID id.ID[project.Project]) device.Device {
	t.Helper()
	created, err := device.CreateDevice(repo.NextID(), projectID, mustDeviceName(t, "device-"+uuid.NewString()), "", device.ProtocolSimulation, device.EmptyDeviceConfig())
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	saved, err := repo.Save(context.Background(), created)
	if err != nil {
		t.Fatalf("Save device: %v", err)
	}
	return saved
}

// mustSaveEnvironment stores a new Environment of the project.
func mustSaveEnvironment(t *testing.T, repo device.EnvironmentRepository, projectID id.ID[project.Project]) device.Environment {
	t.Helper()
	created, err := device.CreateEnvironment(repo.NextID(), projectID, mustEnvironmentName(t, "env-"+uuid.NewString()), "")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	saved, err := repo.Save(context.Background(), created)
	if err != nil {
		t.Fatalf("Save environment: %v", err)
	}
	return saved
}

func mustSavePair(t *testing.T, repo device.EnvironmentDeviceRepository, projectID id.ID[project.Project], environmentID id.ID[device.Environment], deviceID id.ID[device.Device]) device.EnvironmentDevice {
	t.Helper()
	params, err := device.NewEnvironmentDeviceParams([]byte(`{"host":"localhost"}`))
	if err != nil {
		t.Fatalf("NewEnvironmentDeviceParams: %v", err)
	}
	saved, err := repo.Save(context.Background(), device.CreateEnvironmentDevice(projectID, environmentID, deviceID, params))
	if err != nil {
		t.Fatalf("Save pair: %v", err)
	}
	return saved
}

func TestDeviceRepository_CRUDAndProjectScope(t *testing.T) {
	cleanDevices(t)
	ctx := context.Background()
	projects := repositories.NewProjectRepositoryPostgres(testDB)
	other := mustCreateProject(t, projects, "Devices Other "+uuid.NewString())
	repo := repositories.NewDeviceRepositoryPostgres(testDB, mainProject)
	otherRepo := repositories.NewDeviceRepositoryPostgres(testDB, other.ID())

	created, err := device.CreateDevice(repo.NextID(), mainProject, mustDeviceName(t, "pump"), "the pump", device.ProtocolSimulation, device.EmptyDeviceConfig())
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	saved, err := repo.Save(ctx, created)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Version() != version.Committed[device.Device]() || len(saved.PendingEvents()) != 0 {
		t.Errorf("saved %s", saved)
	}
	byID, err := repo.FindByID(ctx, saved.ID())
	if err != nil || byID.Name() != saved.Name() || byID.ProjectID() != mainProject || byID.Config().String() != "{}" {
		t.Errorf("FindByID: %s, %v", byID, err)
	}
	if byName, err := repo.FindByName(ctx, saved.Name()); err != nil || byName.ID() != saved.ID() {
		t.Errorf("FindByName: %s, %v", byName, err)
	}
	if _, err := otherRepo.FindByID(ctx, saved.ID()); !errors.Is(err, device.ErrDeviceNotFound) {
		t.Errorf("through another project: %v, want ErrDeviceNotFound", err)
	}
	if _, err := otherRepo.Save(ctx, saved); err == nil {
		t.Error("a device of another project was saved")
	}

	// A taken name is refused.
	duplicate, _ := device.CreateDevice(repo.NextID(), mainProject, saved.Name(), "", device.ProtocolSimulation, device.EmptyDeviceConfig())
	if _, err := repo.Save(ctx, duplicate); !errors.Is(err, device.ErrDeviceNameTaken) {
		t.Errorf("a taken name: %v, want ErrDeviceNameTaken", err)
	}

	// An update raises the version; a stale one is a conflict.
	renamed, err := saved.Update(saved.Version(), mustDeviceName(t, "pump-2"), "renamed", device.ProtocolSimulation, device.EmptyDeviceConfig())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repo.Save(ctx, renamed)
	if err != nil || updated.Version() != saved.Version().Next() || updated.Name().String() != "pump-2" {
		t.Fatalf("Save update: %s, %v", updated, err)
	}
	if _, err := repo.Save(ctx, renamed); !errors.Is(err, device.ErrDeviceConflict) {
		t.Errorf("a stale write: %v, want ErrDeviceConflict", err)
	}

	// FindAll is scoped and in creation order.
	mustSaveDevice(t, repo, mainProject)
	mustSaveDevice(t, otherRepo, other.ID())
	all, err := repo.FindAll(ctx)
	if err != nil || len(all) != 2 || all[0].ID() != saved.ID() {
		t.Errorf("FindAll: %v, %v", all, err)
	}
}

func TestDeviceRepository_DeleteRefusesAReferencedDevice(t *testing.T) {
	cleanDevices(t)
	ctx := context.Background()
	repo := repositories.NewDeviceRepositoryPostgres(testDB, mainProject)
	envRepo := repositories.NewEnvironmentRepositoryPostgres(testDB, mainProject)
	pairs := repositories.NewEnvironmentDeviceRepositoryPostgres(testDB, mainProject)
	tags := repositories.NewTagRepositoryPostgres(testDB, mainProject)

	// No references: removed.
	free := mustSaveDevice(t, repo, mainProject)
	if err := repo.Delete(ctx, free.Delete()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, free.ID()); !errors.Is(err, device.ErrDeviceNotFound) {
		t.Errorf("after Delete: %v, want ErrDeviceNotFound", err)
	}
	if err := repo.Delete(ctx, free.Delete()); !errors.Is(err, device.ErrDeviceNotFound) {
		t.Errorf("a second Delete: %v, want ErrDeviceNotFound", err)
	}

	// A Tag supplied by the device refuses the removal.
	supplied := mustSaveDevice(t, repo, mainProject)
	tagName, _ := tag.NewTagName("supplied-" + uuid.NewString())
	aTag, _ := tag.CreateTag(tags.NextID(), mainProject, tagName, tag.TagTypeInteger, tag.SuppliedByDevice(supplied.ID()))
	aTag, err := tags.Save(ctx, aTag)
	if err != nil {
		t.Fatalf("Save tag: %v", err)
	}
	if err := repo.Delete(ctx, supplied.Delete()); !errors.Is(err, device.ErrDeviceInUse) {
		t.Errorf("a device a tag is supplied by: %v, want ErrDeviceInUse", err)
	}
	if _, err := repo.FindByID(ctx, supplied.ID()); err != nil {
		t.Errorf("the refused removal must leave the device: %v", err)
	}
	if err := tags.Delete(ctx, aTag.Delete()); err != nil {
		t.Fatalf("Delete tag: %v", err)
	}
	if err := repo.Delete(ctx, supplied.Delete()); err != nil {
		t.Errorf("after the tag is gone: %v", err)
	}

	// A pair holding the device refuses it too.
	inPair := mustSaveDevice(t, repo, mainProject)
	pairEnv := mustSaveEnvironment(t, envRepo, mainProject)
	mustSavePair(t, pairs, mainProject, pairEnv.ID(), inPair.ID())
	if err := repo.Delete(ctx, inPair.Delete()); !errors.Is(err, device.ErrDeviceInUse) {
		t.Errorf("a device in a pair: %v, want ErrDeviceInUse", err)
	}
}

func TestEnvironmentRepository_CRUDAndInUse(t *testing.T) {
	cleanDevices(t)
	ctx := context.Background()
	repo := repositories.NewEnvironmentRepositoryPostgres(testDB, mainProject)
	devices := repositories.NewDeviceRepositoryPostgres(testDB, mainProject)
	pairs := repositories.NewEnvironmentDeviceRepositoryPostgres(testDB, mainProject)

	created, err := device.CreateEnvironment(repo.NextID(), mainProject, mustEnvironmentName(t, "bench"), "the bench")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	saved, err := repo.Save(ctx, created)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	byName, err := repo.FindByName(ctx, saved.Name())
	if err != nil || byName.ID() != saved.ID() {
		t.Errorf("FindByName: %s, %v", byName, err)
	}
	duplicate, _ := device.CreateEnvironment(repo.NextID(), mainProject, saved.Name(), "")
	if _, err := repo.Save(ctx, duplicate); !errors.Is(err, device.ErrEnvironmentNameTaken) {
		t.Errorf("a taken name: %v, want ErrEnvironmentNameTaken", err)
	}
	renamed, _ := saved.Update(saved.Version(), mustEnvironmentName(t, "bench-2"), "renamed")
	updated, err := repo.Save(ctx, renamed)
	if err != nil || updated.Version() != saved.Version().Next() {
		t.Fatalf("Save update: %s, %v", updated, err)
	}
	if _, err := repo.Save(ctx, renamed); !errors.Is(err, device.ErrEnvironmentConflict) {
		t.Errorf("a stale write: %v, want ErrEnvironmentConflict", err)
	}
	if all, err := repo.FindAll(ctx); err != nil || len(all) != 1 || all[0].ID() != saved.ID() {
		t.Errorf("FindAll: %v, %v", all, err)
	}

	// A pair refuses the removal; without one it goes.
	inPair := mustSaveDevice(t, devices, mainProject)
	mustSavePair(t, pairs, mainProject, saved.ID(), inPair.ID())
	if err := repo.Delete(ctx, updated.Delete()); !errors.Is(err, device.ErrEnvironmentInUse) {
		t.Errorf("an environment holding a pair: %v, want ErrEnvironmentInUse", err)
	}
	if err := pairs.Delete(ctx, device.ReconstituteEnvironmentDevice(mainProject, saved.ID(), inPair.ID(), device.EmptyEnvironmentDeviceParams(), version.Committed[device.EnvironmentDevice]())); err != nil {
		t.Fatalf("Delete pair: %v", err)
	}
	updated, _ = repo.FindByID(ctx, saved.ID())
	if err := repo.Delete(ctx, updated.Delete()); err != nil {
		t.Errorf("after the pair is gone: %v", err)
	}
}

func TestEnvironmentDeviceRepository_CRUDAndProjectScope(t *testing.T) {
	cleanDevices(t)
	ctx := context.Background()
	devices := repositories.NewDeviceRepositoryPostgres(testDB, mainProject)
	environments := repositories.NewEnvironmentRepositoryPostgres(testDB, mainProject)
	repo := repositories.NewEnvironmentDeviceRepositoryPostgres(testDB, mainProject)

	envA, envB := mustSaveEnvironment(t, environments, mainProject), mustSaveEnvironment(t, environments, mainProject)
	devA, devB := mustSaveDevice(t, devices, mainProject), mustSaveDevice(t, devices, mainProject)

	saved := mustSavePair(t, repo, mainProject, envA.ID(), devA.ID())
	if saved.Version() != version.Committed[device.EnvironmentDevice]() || len(saved.PendingEvents()) != 0 {
		t.Errorf("saved %s", saved)
	}
	if found, err := repo.FindByPair(ctx, envA.ID(), devA.ID()); err != nil || !strings.Contains(found.Params().String(), "localhost") {
		t.Errorf("FindByPair: %s, %v", found, err)
	}
	mustSavePair(t, repo, mainProject, envA.ID(), devB.ID())
	mustSavePair(t, repo, mainProject, envB.ID(), devA.ID())
	if all, err := repo.FindAll(ctx); err != nil || len(all) != 3 {
		t.Errorf("FindAll: %v, %v", all, err)
	}
	if ofA, err := repo.FindByEnvironment(ctx, envA.ID()); err != nil || len(ofA) != 2 || ofA[0].DeviceID() != devA.ID() {
		t.Errorf("FindByEnvironment: %v, %v", ofA, err)
	}
	if none, err := repo.FindByEnvironment(ctx, id.NewID[device.Environment]()); !errors.Is(err, device.ErrEnvironmentNotFound) {
		t.Errorf("an absent environment: %v, %v, want ErrEnvironmentNotFound", none, err)
	}

	// The pair is its two sides: a second insert is refused.
	if _, err := repo.Save(ctx, device.CreateEnvironmentDevice(mainProject, envA.ID(), devA.ID(), device.EmptyEnvironmentDeviceParams())); !errors.Is(err, device.ErrEnvironmentDeviceExists) {
		t.Errorf("a taken pair: %v, want ErrEnvironmentDeviceExists", err)
	}

	// An update raises the version; a stale one is a conflict.
	params, _ := device.NewEnvironmentDeviceParams([]byte(`{"host":"10.0.0.1"}`))
	changed, err := saved.UpdateParams(saved.Version(), params)
	if err != nil {
		t.Fatalf("UpdateParams: %v", err)
	}
	updated, err := repo.Save(ctx, changed)
	if err != nil || updated.Version() != saved.Version().Next() || !strings.Contains(updated.Params().String(), "10.0.0.1") {
		t.Fatalf("Save update: %s, %v", updated, err)
	}
	if _, err := repo.Save(ctx, changed); !errors.Is(err, device.ErrEnvironmentDeviceConflict) {
		t.Errorf("a stale write: %v, want ErrEnvironmentDeviceConflict", err)
	}

	// Sides of another project are not found.
	other := mustCreateProject(t, repositories.NewProjectRepositoryPostgres(testDB), "Pairs Other "+uuid.NewString())
	foreignEnv := mustSaveEnvironment(t, repositories.NewEnvironmentRepositoryPostgres(testDB, other.ID()), other.ID())
	foreignDev := mustSaveDevice(t, repositories.NewDeviceRepositoryPostgres(testDB, other.ID()), other.ID())
	if _, err := repo.Save(ctx, device.CreateEnvironmentDevice(mainProject, foreignEnv.ID(), devA.ID(), device.EmptyEnvironmentDeviceParams())); !errors.Is(err, device.ErrEnvironmentNotFound) {
		t.Errorf("an environment of another project: %v, want ErrEnvironmentNotFound", err)
	}
	if _, err := repo.Save(ctx, device.CreateEnvironmentDevice(mainProject, envA.ID(), foreignDev.ID(), device.EmptyEnvironmentDeviceParams())); !errors.Is(err, device.ErrDeviceNotFound) {
		t.Errorf("a device of another project: %v, want ErrDeviceNotFound", err)
	}
	if _, err := repo.FindByPair(ctx, foreignEnv.ID(), foreignDev.ID()); !errors.Is(err, device.ErrEnvironmentDeviceNotFound) {
		t.Errorf("a pair of another project: %v, want ErrEnvironmentDeviceNotFound", err)
	}

	// Removal is version-checked; a pair deleted meanwhile is not found.
	if err := repo.Delete(ctx, updated.Delete()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByPair(ctx, envA.ID(), devA.ID()); !errors.Is(err, device.ErrEnvironmentDeviceNotFound) {
		t.Errorf("after Delete: %v, want ErrEnvironmentDeviceNotFound", err)
	}
}

func TestTagRepository_SupplierMustBeOfTheProject(t *testing.T) {
	cleanDevices(t)
	ctx := context.Background()
	tags := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	other := mustCreateProject(t, repositories.NewProjectRepositoryPostgres(testDB), "Tags Other "+uuid.NewString())
	foreign := mustSaveDevice(t, repositories.NewDeviceRepositoryPostgres(testDB, other.ID()), other.ID())

	name, _ := tag.NewTagName("supplied-" + uuid.NewString())
	withForeign, _ := tag.CreateTag(tags.NextID(), mainProject, name, tag.TagTypeInteger, tag.SuppliedByDevice(foreign.ID()))
	if _, err := tags.Save(ctx, withForeign); !errors.Is(err, tag.ErrSupplierOfAnotherProject) {
		t.Errorf("a supplier of another project: %v, want ErrSupplierOfAnotherProject", err)
	}

	mine := mustSaveDevice(t, repositories.NewDeviceRepositoryPostgres(testDB, mainProject), mainProject)
	withMine, _ := tag.CreateTag(tags.NextID(), mainProject, name, tag.TagTypeInteger, tag.SuppliedByDevice(mine.ID()))
	saved, err := tags.Save(ctx, withMine)
	if err != nil {
		t.Fatalf("a supplier of the project: %v", err)
	}
	found, err := tags.FindByID(ctx, saved.ID())
	if err != nil || found.Supplier() != tag.SuppliedByDevice(mine.ID()) {
		t.Errorf("the stored supplier: %s, %v", found.Supplier(), err)
	}
}
