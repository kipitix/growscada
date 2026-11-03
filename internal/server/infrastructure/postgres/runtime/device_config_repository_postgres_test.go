package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/project"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
)

// TestFindConfig_ReadsTheRunningRevisionsCopies: the Gateway's configuration
// is the Device and the Environment of the Revision, their pair's parameters
// and the Tags the Device supplies there.
func TestFindConfig_ReadsTheRunningRevisionsCopies(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	ctx := context.Background()

	deviceID, environmentID := uuid.New(), uuid.New()
	otherDeviceID := uuid.New()
	supplied, alsoSupplied, foreign := uuid.New(), uuid.New(), uuid.New()
	file := documentOf(projectID,
		withDevices(
			projectv0.Device{ID: deviceID, Name: "sim", Protocol: "simulation", Config: projectv0.JSONObject(`{"patterns":[]}`)},
			projectv0.Device{ID: otherDeviceID, Name: "sim-2", Protocol: "simulation"},
		),
		withEnvironments(projectv0.Environment{ID: environmentID, Name: "bench"}),
		withEnvironmentDevices(projectv0.EnvironmentDevice{
			EnvironmentID: environmentID, DeviceID: deviceID,
			Params: projectv0.JSONObject(`{"address":"10.0.0.1:502"}`),
		}),
		withTags(
			projectv0.TagDefinition{ID: supplied, Name: "speed", Type: "integer", SupplierDeviceID: &deviceID},
			projectv0.TagDefinition{ID: alsoSupplied, Name: "running", Type: "boolean", SupplierDeviceID: &deviceID},
			projectv0.TagDefinition{ID: foreign, Name: "other", Type: "integer", SupplierDeviceID: &otherDeviceID},
		),
	)
	mustCreateRevision(t, runtimepg.NewRevisionRepositoryPostgres(db), projectID.UUID(), file, time.Now())

	repo := runtimepg.NewDeviceConfigRepositoryPostgres(db)
	config, err := repo.FindConfig(ctx, projectID.UUID(), deviceID, environmentID)
	if err != nil {
		t.Fatalf("FindConfig: %v", err)
	}
	if config.Device.ID != deviceID || config.Device.Name != "sim" || config.Device.Protocol != "simulation" {
		t.Errorf("config.Device = %+v", config.Device)
	}
	if !sameJSON(t, config.Device.Config, `{"patterns":[]}`) {
		t.Errorf("config.Device.Config = %s", config.Device.Config)
	}
	if config.Environment.ID != environmentID || config.Environment.Name != "bench" {
		t.Errorf("config.Environment = %+v", config.Environment)
	}
	if !sameJSON(t, config.Connection, `{"address":"10.0.0.1:502"}`) {
		t.Errorf("config.Connection = %s", config.Connection)
	}
	if len(config.Tags) != 2 {
		t.Fatalf("config.Tags = %+v, want the two tags of the device", config.Tags)
	}
	got := map[uuid.UUID]string{config.Tags[0].ID: config.Tags[0].Name, config.Tags[1].ID: config.Tags[1].Name}
	if got[supplied] != "speed" || got[alsoSupplied] != "running" {
		t.Errorf("config.Tags = %+v", config.Tags)
	}
}

// TestFindConfig_OutsideTheRevision: a Device or an Environment the running
// Revision does not have is not found, nor is a pair of a Device that does
// not stand in the Environment.
func TestFindConfig_OutsideTheRevision(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	ctx := context.Background()
	repo := runtimepg.NewDeviceConfigRepositoryPostgres(db)
	revisions := runtimepg.NewRevisionRepositoryPostgres(db)

	deviceID, environmentID := uuid.New(), uuid.New()
	file := documentOf(projectID,
		withDevices(projectv0.Device{ID: deviceID, Name: "sim", Protocol: "simulation"}),
		withEnvironments(projectv0.Environment{ID: environmentID, Name: "bench"}),
	)
	mustCreateRevision(t, revisions, projectID.UUID(), file, time.Now())

	if _, err := repo.FindConfig(ctx, projectID.UUID(), uuid.New(), environmentID); !errors.Is(err, device.ErrDeviceNotFound) {
		t.Errorf("an unknown device: %v, want ErrDeviceNotFound", err)
	}
	if _, err := repo.FindConfig(ctx, projectID.UUID(), deviceID, uuid.New()); !errors.Is(err, device.ErrEnvironmentNotFound) {
		t.Errorf("an unknown environment: %v, want ErrEnvironmentNotFound", err)
	}
	if _, err := repo.FindConfig(ctx, projectID.UUID(), deviceID, environmentID); !errors.Is(err, device.ErrEnvironmentDeviceNotFound) {
		t.Errorf("a device outside the environment: %v, want ErrEnvironmentDeviceNotFound", err)
	}

	// The next Revision drops the Device: its copy goes, and a dormant token's
	// Device is not found.
	mustCreateRevision(t, revisions, projectID.UUID(), documentOf(projectID), time.Now())
	if _, err := repo.FindConfig(ctx, projectID.UUID(), deviceID, environmentID); !errors.Is(err, device.ErrDeviceNotFound) {
		t.Errorf("a device outside the running revision: %v, want ErrDeviceNotFound", err)
	}
}

// TestFindConfig_OfADeletedProject_IsProjectNotFound: a Project marked deleted
// stops serving its Gateway's configuration at once, even while the copies it
// leaves behind and the Device Token outlive the mark until the cleanup.
func TestFindConfig_OfADeletedProject_IsProjectNotFound(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	ctx := context.Background()

	deviceID, environmentID := uuid.New(), uuid.New()
	mustCreateRevision(t, runtimepg.NewRevisionRepositoryPostgres(db), projectID.UUID(),
		documentOf(projectID,
			withDevices(projectv0.Device{ID: deviceID, Name: "sim", Protocol: "simulation"}),
			withEnvironments(projectv0.Environment{ID: environmentID, Name: "bench"}),
			withEnvironmentDevices(projectv0.EnvironmentDevice{EnvironmentID: environmentID, DeviceID: deviceID}),
		), time.Now())

	repo := runtimepg.NewDeviceConfigRepositoryPostgres(db)
	if _, err := repo.FindConfig(ctx, projectID.UUID(), deviceID, environmentID); err != nil {
		t.Fatalf("FindConfig of a live project: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE projects SET deleted_at = now() WHERE id = $1`, projectID.UUID()); err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	if _, err := repo.FindConfig(ctx, projectID.UUID(), deviceID, environmentID); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("a deleted project: %v, want ErrProjectNotFound", err)
	}
}

// sameJSON reports whether got and want are the same JSON value, whatever
// their spacing: jsonb stores a normalized form.
func sameJSON(t *testing.T, got []byte, want string) bool {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode %s: %v", want, err)
	}
	return reflect.DeepEqual(gotValue, wantValue)
}
