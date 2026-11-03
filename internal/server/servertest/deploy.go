package servertest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
)

// DeployProject stores the Draft of the Project as its first Revision, as the
// Engineering node's Deploy will (task 35, part 4): the Tags, the Devices, the
// Environments and their pairs, read from the engineering tables. A test that
// needs a Gateway's configuration or a Tag value of the running configuration
// deploys in its setup; until then the bridge of the Draft fills the copies.
func DeployProject(t *testing.T, db *sql.DB, projectID uuid.UUID) {
	t.Helper()
	file := ProjectFileOf(t, db, projectID)
	// The same service the server wires: a Deploy that names another Project
	// is refused, and the event the service publishes has no readers here.
	service := runtimeapp.NewRevisionService(runtimepg.NewRevisionRepositoryPostgres(db), func(event.Event) {})
	if _, err := service.Deploy(context.Background(), projectID, file); err != nil {
		t.Fatalf("deploy project %s: %v", projectID, err)
	}
}

// ProjectFileOf reads the Project's Draft from the engineering tables: the
// part of a ProjectFile a Gateway or a Tag write needs. Scenes and WidgetTypes
// are left out: no test helper deploys them yet.
func ProjectFileOf(t *testing.T, db *sql.DB, projectID uuid.UUID) projectv0.ProjectFile {
	t.Helper()
	file := projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID},
	}

	readRows(t, db, projectID, "tags",
		`SELECT id, name, type, device_id FROM engineering.tags WHERE project_id = $1`,
		func(rows *sql.Rows) {
			var (
				def      projectv0.TagDefinition
				deviceID *uuid.UUID
			)
			if err := rows.Scan(&def.ID, &def.Name, &def.Type, &deviceID); err != nil {
				t.Fatalf("scan a tag of project %s: %v", projectID, err)
			}
			def.SupplierDeviceID = deviceID
			file.Tags = append(file.Tags, def)
		})

	readRows(t, db, projectID, "devices",
		`SELECT id, name, comment, protocol, config FROM engineering.devices WHERE project_id = $1`,
		func(rows *sql.Rows) {
			var (
				device projectv0.Device
				config []byte
			)
			if err := rows.Scan(&device.ID, &device.Name, &device.Comment, &device.Protocol, &config); err != nil {
				t.Fatalf("scan a device of project %s: %v", projectID, err)
			}
			device.Config = projectv0.JSONObject(config)
			file.Devices = append(file.Devices, device)
		})

	readRows(t, db, projectID, "environments",
		`SELECT id, name, comment FROM engineering.environments WHERE project_id = $1`,
		func(rows *sql.Rows) {
			var environment projectv0.Environment
			if err := rows.Scan(&environment.ID, &environment.Name, &environment.Comment); err != nil {
				t.Fatalf("scan an environment of project %s: %v", projectID, err)
			}
			file.Environments = append(file.Environments, environment)
		})

	readRows(t, db, projectID, "environment devices",
		`SELECT ed.environment_id, ed.device_id, ed.params
		 FROM engineering.environment_devices ed
		 JOIN engineering.environments e ON e.id = ed.environment_id
		 WHERE e.project_id = $1`,
		func(rows *sql.Rows) {
			var (
				pair   projectv0.EnvironmentDevice
				params []byte
			)
			if err := rows.Scan(&pair.EnvironmentID, &pair.DeviceID, &params); err != nil {
				t.Fatalf("scan an environment device of project %s: %v", projectID, err)
			}
			pair.Params = projectv0.JSONObject(params)
			file.EnvironmentDevices = append(file.EnvironmentDevices, pair)
		})

	return file
}

// readRows runs query for the Project and hands every row to scan, closing the
// rows whatever scan does.
func readRows(t *testing.T, db *sql.DB, projectID uuid.UUID, what, query string, scan func(*sql.Rows)) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), query, projectID)
	if err != nil {
		t.Fatalf("read the %s of project %s: %v", what, projectID, err)
	}
	defer rows.Close()
	for rows.Next() {
		scan(rows)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate over the %s of project %s: %v", what, projectID, err)
	}
}
