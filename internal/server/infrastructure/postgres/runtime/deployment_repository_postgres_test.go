package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/tagstate"
	runtimepg "github.com/kipitix/growscada/internal/server/infrastructure/postgres/runtime"
)

// documentOf builds a ProjectFile of the Project with the given parts filled
// in; anything not passed stays empty.
func documentOf(projectID id.ID[project.Project], parts ...func(*projectv0.ProjectFile)) projectv0.ProjectFile {
	file := projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID.UUID()},
	}
	for _, part := range parts {
		part(&file)
	}
	return file
}

func withTags(defs ...projectv0.TagDefinition) func(*projectv0.ProjectFile) {
	return func(f *projectv0.ProjectFile) { f.Tags = defs }
}

func withDevices(devices ...projectv0.Device) func(*projectv0.ProjectFile) {
	return func(f *projectv0.ProjectFile) { f.Devices = devices }
}

func withEnvironments(environments ...projectv0.Environment) func(*projectv0.ProjectFile) {
	return func(f *projectv0.ProjectFile) { f.Environments = environments }
}

func withEnvironmentDevices(pairs ...projectv0.EnvironmentDevice) func(*projectv0.ProjectFile) {
	return func(f *projectv0.ProjectFile) { f.EnvironmentDevices = pairs }
}

func withLibraryPins(pins ...projectv0.LibraryPin) func(*projectv0.ProjectFile) {
	return func(f *projectv0.ProjectFile) { f.LibraryPins = pins }
}

func mustCreateDeployment(t *testing.T, repo runtimeapp.DeploymentRepository, projectID uuid.UUID, file projectv0.ProjectFile, at time.Time) runtimeapp.Deployment {
	t.Helper()
	deployment, err := repo.CreateDeployment(context.Background(), projectID, file, runtimeapp.DeploymentCauseDeploy, nil, at)
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	return deployment
}

func mustFindCurrent(t *testing.T, repo runtimeapp.DeploymentRepository, projectID uuid.UUID) (runtimeapp.Deployment, bool) {
	t.Helper()
	found, ok, err := repo.FindCurrent(context.Background(), projectID)
	if err != nil {
		t.Fatalf("FindCurrent: %v", err)
	}
	return found, ok
}

func countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var count int
	if err := postgres.DB(t).QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return count
}

func TestCreateDeployment_NumbersDeploymentsAndKeepsTheDocument(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	if _, ok := mustFindCurrent(t, repo, projectID.UUID()); ok {
		t.Fatal("a project that was never deployed has a deployment")
	}

	tagID := uuid.New()
	first := documentOf(projectID, withTags(projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer"}))
	start := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		file := first
		file.Tags = []projectv0.TagDefinition{{ID: tagID, Name: "speed", Type: "integer"}}
		at := start.Add(time.Duration(i) * time.Minute)
		deployment, err := repo.CreateDeployment(ctx, projectID.UUID(), file, runtimeapp.DeploymentCauseDeploy, nil, at)
		if err != nil {
			t.Fatalf("CreateDeployment %d: %v", i, err)
		}
		if deployment.Number != i {
			t.Errorf("deployment %d created with number %d", i, deployment.Number)
		}
		if !deployment.DeployedAt.Equal(at) {
			t.Errorf("deployment %d deployed at %s, want %s", i, deployment.DeployedAt, at)
		}
		if !reflect.DeepEqual(deployment.Content, file) {
			t.Errorf("deployment %d content differs from the document", i)
		}
	}

	current, ok := mustFindCurrent(t, repo, projectID.UUID())
	if !ok {
		t.Fatal("a deployed project has no deployment")
	}
	if current.Number != 3 {
		t.Errorf("current deployment %d, want 3", current.Number)
	}
	if !current.DeployedAt.Equal(start.Add(3 * time.Minute)) {
		t.Errorf("current deployment deployed at %s", current.DeployedAt)
	}
	if !reflect.DeepEqual(current.Content.Tags, first.Tags) || current.Content.ProjectID != projectID.UUID() {
		t.Errorf("current deployment content %+v, want %+v", current.Content, first)
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.deployments WHERE project_id = $1`, projectID.UUID()); got != 3 {
		t.Errorf("stored deployments %d, want 3", got)
	}
}

func TestCreateDeployment_ConcurrentDeploysTakeDistinctNumbers(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	const deploys = 8
	numbers := make([]int, deploys)
	errs := make([]error, deploys)
	var wg sync.WaitGroup
	for i := 0; i < deploys; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			deployment, err := repo.CreateDeployment(ctx, projectID.UUID(), documentOf(projectID), runtimeapp.DeploymentCauseDeploy, nil, time.Now())
			numbers[i], errs[i] = deployment.Number, err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("deploy %d: %v", i, err)
		}
	}
	sort.Ints(numbers)
	for i, number := range numbers {
		if number != i+1 {
			t.Fatalf("numbers %v, want 1..%d", numbers, deploys)
		}
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.deployments WHERE project_id = $1`, projectID.UUID()); got != deploys {
		t.Errorf("stored deployments %d, want %d", got, deploys)
	}
	if current, ok := mustFindCurrent(t, repo, projectID.UUID()); !ok || current.Number != deploys {
		t.Errorf("current deployment %+v (%v), want %d", current.Number, ok, deploys)
	}
}

func TestCreateDeployment_TheUniqueKeyIsTheSafetyNet(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID), time.Now())

	_, err := postgres.DB(t).ExecContext(context.Background(),
		`INSERT INTO runtime.deployments (project_id, number, deployed_at, cause, config) VALUES ($1, 1, now(), 'deploy', '{}')`,
		projectID.UUID(),
	)
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || pqErr.Code != "23505" {
		t.Fatalf("a second deployment 1: %v, want a unique violation", err)
	}
}

func TestCreateDeployment_KeepsStatesAndTokensAndDropsAbsentCopies(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	ctx := context.Background()
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	states := runtimepg.NewTagStateRepositoryPostgres(db)

	keptTag, droppedTag := uuid.New(), uuid.New()
	deviceID, environmentID := uuid.New(), uuid.New()
	first := documentOf(projectID,
		withTags(
			projectv0.TagDefinition{ID: keptTag, Name: "kept", Type: "integer"},
			projectv0.TagDefinition{ID: droppedTag, Name: "dropped", Type: "integer"},
		),
		withDevices(projectv0.Device{ID: deviceID, Name: "meter", Protocol: "simulation", Config: projectv0.JSONObject(`{"host": "meter"}`)}),
		withEnvironments(projectv0.Environment{ID: environmentID, Name: "bench"}),
		withEnvironmentDevices(projectv0.EnvironmentDevice{
			EnvironmentID: environmentID, DeviceID: deviceID,
			Params: projectv0.JSONObject(`{"address": "10.0.0.1"}`),
		}),
	)
	mustCreateDeployment(t, repo, projectID.UUID(), first, time.Now())

	writeValue(t, states, projectID, keptTag, 42)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO runtime.device_tokens (device_id, project_id, token_hash, fingerprint, issued_at)
		 VALUES ($1, $2, 'hash', 'gsc_dev_…', now())`,
		deviceID, projectID.UUID(),
	); err != nil {
		t.Fatalf("insert token: %v", err)
	}

	second := documentOf(projectID, withTags(projectv0.TagDefinition{ID: keptTag, Name: "kept", Type: "integer"}))
	mustCreateDeployment(t, repo, projectID.UUID(), second, time.Now())

	if got := findValue(t, states, projectID, keptTag); got != "42" {
		t.Errorf("the state of the kept tag is %q, want 42", got)
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.tags WHERE id = $1`, droppedTag); got != 0 {
		t.Errorf("the copy of the dropped tag is still stored")
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.tag_states WHERE tag_id = $1`, droppedTag); got != 0 {
		t.Errorf("the state of the dropped tag is still stored")
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.devices WHERE id = $1`, deviceID); got != 0 {
		t.Errorf("the copy of the dropped device is still stored")
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.environments WHERE id = $1`, environmentID); got != 0 {
		t.Errorf("the copy of the dropped environment is still stored")
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.environment_devices WHERE device_id = $1`, deviceID); got != 0 {
		t.Errorf("the pair of the dropped device is still stored")
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.device_tokens WHERE device_id = $1`, deviceID); got != 1 {
		t.Errorf("the token of the dropped device is gone: a token outlives a deploy")
	}

	third := documentOf(projectID,
		withDevices(projectv0.Device{ID: deviceID, Name: "meter", Protocol: "simulation", Config: projectv0.JSONObject(`{"host": "meter"}`)}),
		withEnvironments(projectv0.Environment{ID: environmentID, Name: "bench"}),
		withEnvironmentDevices(projectv0.EnvironmentDevice{
			EnvironmentID: environmentID, DeviceID: deviceID,
			Params: projectv0.JSONObject(`{"address": "10.0.0.2"}`),
		}),
	)
	mustCreateDeployment(t, repo, projectID.UUID(), third, time.Now())

	var params []byte
	if err := db.QueryRowContext(ctx,
		`SELECT params FROM runtime.environment_devices WHERE environment_id = $1 AND device_id = $2`,
		environmentID, deviceID,
	).Scan(&params); err != nil {
		t.Fatalf("read the pair params: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(params, &decoded); err != nil {
		t.Fatalf("decode the pair params: %v", err)
	}
	if decoded["address"] != "10.0.0.2" {
		t.Errorf("the pair params are %s, want the address of the new deployment", params)
	}
}

func TestCreateDeployment_AnEmptyConfigAndParamsAreTheEmptyObject(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	deviceID, environmentID := uuid.New(), uuid.New()

	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withDevices(projectv0.Device{ID: deviceID, Name: "meter", Protocol: "simulation"}),
		withEnvironments(projectv0.Environment{ID: environmentID, Name: "bench"}),
		withEnvironmentDevices(projectv0.EnvironmentDevice{EnvironmentID: environmentID, DeviceID: deviceID}),
	), time.Now())

	ctx := context.Background()
	var config, params []byte
	if err := db.QueryRowContext(ctx, `SELECT config FROM runtime.devices WHERE id = $1`, deviceID).Scan(&config); err != nil {
		t.Fatalf("read the device config: %v", err)
	}
	if string(config) != "{}" {
		t.Errorf("the config of a device that carries none is %s, want {}", config)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT params FROM runtime.environment_devices WHERE device_id = $1`, deviceID,
	).Scan(&params); err != nil {
		t.Fatalf("read the pair params: %v", err)
	}
	if string(params) != "{}" {
		t.Errorf("the params of a pair that carries none are %s, want {}", params)
	}
}

func TestCreateDeployment_RefusesAPairWhoseCopiesAreOfAnotherProject(t *testing.T) {
	projectID, orgID := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	ctx := context.Background()

	otherID := id.NewID[project.Project]()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO projects (id, organization_id, name, version) VALUES ($1, $2, $3, 1)`,
		otherID.UUID(), orgID.UUID(), "p-"+otherID.String()); err != nil {
		t.Fatalf("insert the other project: %v", err)
	}
	deviceID, environmentID := uuid.New(), uuid.New()
	mustCreateDeployment(t, repo, otherID.UUID(), documentOf(otherID,
		withDevices(projectv0.Device{ID: deviceID, Name: "meter", Protocol: "simulation", Config: projectv0.JSONObject(`{"host": "meter"}`)}),
		withEnvironments(projectv0.Environment{ID: environmentID, Name: "bench"}),
		withEnvironmentDevices(projectv0.EnvironmentDevice{
			EnvironmentID: environmentID, DeviceID: deviceID,
			Params: projectv0.JSONObject(`{"address": "10.0.0.1"}`),
		}),
	), time.Now())

	_, err := repo.CreateDeployment(ctx, projectID.UUID(), documentOf(projectID,
		withEnvironmentDevices(projectv0.EnvironmentDevice{
			EnvironmentID: environmentID, DeviceID: deviceID,
			Params: projectv0.JSONObject(`{"address": "10.9.9.9"}`),
		}),
	), runtimeapp.DeploymentCauseDeploy, nil, time.Now())
	if err == nil {
		t.Fatal("the pair of another project was taken")
	}
	if !strings.Contains(err.Error(), "not of project") {
		t.Errorf("error %v, want a refusal naming the project", err)
	}

	var params []byte
	if err := db.QueryRowContext(ctx,
		`SELECT params FROM runtime.environment_devices WHERE environment_id = $1 AND device_id = $2`,
		environmentID, deviceID,
	).Scan(&params); err != nil {
		t.Fatalf("read the pair params: %v", err)
	}
	if !strings.Contains(string(params), "10.0.0.1") {
		t.Errorf("the params of the other project's pair are %s, want them untouched", params)
	}
}

func TestCreateDeployment_HealsACopyThatLostItsState(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	states := runtimepg.NewTagStateRepositoryPostgres(db)
	tagID := uuid.New()
	file := documentOf(projectID, withTags(projectv0.TagDefinition{ID: tagID, Name: "counter", Type: "integer"}))

	mustCreateDeployment(t, repo, projectID.UUID(), file, time.Now())
	if _, err := db.ExecContext(context.Background(), `DELETE FROM runtime.tag_states WHERE tag_id = $1`, tagID); err != nil {
		t.Fatalf("drop the state: %v", err)
	}
	if _, ok := stateOf(t, states, projectID, tagID); ok {
		t.Fatal("the state survives its deletion")
	}

	mustCreateDeployment(t, repo, projectID.UUID(), file, time.Now())
	state, ok := stateOf(t, states, projectID, tagID)
	if !ok {
		t.Fatal("the deploy of an unchanged document did not give the copy its state back")
	}
	if state.Value().String() != "0" || state.Quality().String() != "bad" {
		t.Errorf("the healed state is %q/%s, want the initial state of an integer tag", state.Value().String(), state.Quality().String())
	}
}

func TestCreateDeployment_TwoEnvironmentsMaySwapTheirNames(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	ctx := context.Background()
	first, second := uuid.New(), uuid.New()

	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withEnvironments(
			projectv0.Environment{ID: first, Name: "bench"},
			projectv0.Environment{ID: second, Name: "lab"},
		),
	), time.Now())

	// The first write takes the name the second one has not left yet.
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withEnvironments(
			projectv0.Environment{ID: first, Name: "lab"},
			projectv0.Environment{ID: second, Name: "bench"},
		),
	), time.Now())

	for environmentID, want := range map[uuid.UUID]string{first: "lab", second: "bench"} {
		var name string
		if err := db.QueryRowContext(ctx, `SELECT name FROM runtime.environments WHERE id = $1`, environmentID).Scan(&name); err != nil {
			t.Fatalf("read the name of environment %s: %v", environmentID, err)
		}
		if name != want {
			t.Errorf("environment %s is %q, want %q", environmentID, name, want)
		}
	}
}

func TestCreateDeployment_RefusesTwoEnvironmentsOfOneName(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))

	_, err := repo.CreateDeployment(context.Background(), projectID.UUID(), documentOf(projectID,
		withEnvironments(
			projectv0.Environment{ID: uuid.New(), Name: "bench"},
			projectv0.Environment{ID: uuid.New(), Name: "bench"},
		),
	), runtimeapp.DeploymentCauseDeploy, nil, time.Now())
	if err == nil {
		t.Fatal("two environments of one name were taken")
	}
}

func TestCreateDeployment_ResetsTheStateOfAChangedTagTypeAndKeepsTheOthers(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	states := runtimepg.NewTagStateRepositoryPostgres(db)

	changed, renamed := uuid.New(), uuid.New()
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withTags(
			projectv0.TagDefinition{ID: changed, Name: "counter", Type: "integer"},
			projectv0.TagDefinition{ID: renamed, Name: "plain", Type: "integer"},
		),
	), time.Now())
	writeValue(t, states, projectID, changed, 42)
	writeValue(t, states, projectID, renamed, 7)

	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withTags(
			projectv0.TagDefinition{ID: changed, Name: "counter", Type: "string"},
			projectv0.TagDefinition{ID: renamed, Name: "renamed", Type: "integer"},
		),
	), time.Now())

	state, ok := stateOf(t, states, projectID, changed)
	if !ok {
		t.Fatal("the state of the retyped tag is gone")
	}
	if state.Value().String() != "" || state.Quality().String() != "bad" {
		t.Errorf("the retyped tag is %q/%s, want the initial state of a string tag", state.Value().String(), state.Quality().String())
	}
	if got := findValue(t, states, projectID, renamed); got != "7" {
		t.Errorf("the state of the renamed tag is %q, want 7: a rename is not a type change", got)
	}
}

func TestCreateDeployment_TheProjectTakesItsDeploymentsAlong(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID), time.Now())

	if _, err := db.ExecContext(context.Background(), `DELETE FROM projects WHERE id = $1`, projectID.UUID()); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.deployments WHERE project_id = $1`, projectID.UUID()); got != 0 {
		t.Errorf("deployments of the deleted project: %d, want none", got)
	}
}

func TestCreateDeployment_CopiesTheSupplyingDeviceOfATag(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	deviceID, tagID := uuid.New(), uuid.New()

	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withDevices(projectv0.Device{ID: deviceID, Name: "meter", Protocol: "simulation"}),
		withTags(projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer", SupplierDeviceID: &deviceID}),
	), time.Now())

	var supplier *uuid.UUID
	if err := db.QueryRowContext(context.Background(), `SELECT device_id FROM runtime.tags WHERE id = $1`, tagID).Scan(&supplier); err != nil {
		t.Fatalf("read the supplier of the tag: %v", err)
	}
	if supplier == nil || *supplier != deviceID {
		t.Errorf("the supplying device of the tag is %v, want %s: the Runtime node refuses a write by any other", supplier, deviceID)
	}
}

func TestCreateDeployment_UpdatesTheCopiesOfAKnownDeviceAndEnvironment(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	deviceID, environmentID := uuid.New(), uuid.New()
	ctx := context.Background()

	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withDevices(projectv0.Device{ID: deviceID, Name: "meter", Comment: "old", Protocol: "simulation", Config: projectv0.JSONObject(`{"host": "a"}`)}),
		withEnvironments(projectv0.Environment{ID: environmentID, Name: "bench", Comment: "old"}),
	), time.Now())

	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withDevices(projectv0.Device{ID: deviceID, Name: "meter-2", Comment: "new", Protocol: "modbus", Config: projectv0.JSONObject(`{"host": "b"}`)}),
		withEnvironments(projectv0.Environment{ID: environmentID, Name: "lab", Comment: "new"}),
	), time.Now())

	var name, comment, protocol string
	var config []byte
	if err := db.QueryRowContext(ctx,
		`SELECT name, comment, protocol, config FROM runtime.devices WHERE id = $1`, deviceID,
	).Scan(&name, &comment, &protocol, &config); err != nil {
		t.Fatalf("read the device copy: %v", err)
	}
	if name != "meter-2" || comment != "new" || protocol != "modbus" || !strings.Contains(string(config), `"b"`) {
		t.Errorf("the device copy is %s/%s/%s/%s, want the values of the new deployment", name, comment, protocol, config)
	}
	var environmentName, environmentComment string
	if err := db.QueryRowContext(ctx,
		`SELECT name, comment FROM runtime.environments WHERE id = $1`, environmentID,
	).Scan(&environmentName, &environmentComment); err != nil {
		t.Fatalf("read the environment copy: %v", err)
	}
	if environmentName != "lab" || environmentComment != "new" {
		t.Errorf("the environment copy is %s/%s, want the values of the new deployment", environmentName, environmentComment)
	}
}

func TestCreateDeployment_RefusesADocumentOfAnotherProject(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	otherID := id.NewID[project.Project]()

	if _, err := repo.CreateDeployment(context.Background(), projectID.UUID(), documentOf(otherID), runtimeapp.DeploymentCauseDeploy, nil, time.Now()); err == nil {
		t.Fatal("the document of another project was stored")
	} else if !strings.Contains(err.Error(), "document of project") {
		t.Errorf("error %v, want a refusal naming the document's project", err)
	}
}

// TestDeploymentNamesLibrary_ReadsTheRunningDeploymentAlone: a pin of a
// Project's running Deployment answers yes; one only of an earlier Deployment,
// or of a Project no longer live, answers no (task 71).
func TestDeploymentNamesLibrary_ReadsTheRunningDeploymentAlone(t *testing.T) {
	projectID, _ := newProject(t)
	otherProjectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	ctx := context.Background()
	start := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	libraryID, otherLibraryID := uuid.New(), uuid.New()

	named, err := repo.DeploymentNamesLibrary(ctx, libraryID)
	if err != nil || named {
		t.Fatalf("before any Deployment: %v, %v", named, err)
	}

	pinned := documentOf(projectID, withLibraryPins(projectv0.LibraryPin{LibraryID: libraryID, Release: 1}))
	mustCreateDeployment(t, repo, projectID.UUID(), pinned, start)
	if named, err := repo.DeploymentNamesLibrary(ctx, libraryID); err != nil || !named {
		t.Fatalf("a pin of the running Deployment: %v, %v", named, err)
	}

	// A later Deployment without the pin makes it history alone.
	plain := documentOf(projectID)
	mustCreateDeployment(t, repo, projectID.UUID(), plain, start.Add(time.Minute))
	if named, err := repo.DeploymentNamesLibrary(ctx, libraryID); err != nil || named {
		t.Fatalf("a pin of an old Deployment: %v, %v", named, err)
	}

	// A Project marked deleted has no running Deployment: its document stops
	// keeping the Library, as its cleanup will remove it.
	otherPinned := documentOf(otherProjectID, withLibraryPins(projectv0.LibraryPin{LibraryID: otherLibraryID, Release: 1}))
	mustCreateDeployment(t, repo, otherProjectID.UUID(), otherPinned, start.Add(2*time.Minute))
	if _, err := db.ExecContext(ctx, `UPDATE projects SET deleted_at = now() WHERE id = $1`, otherProjectID.UUID()); err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	if named, err := repo.DeploymentNamesLibrary(ctx, otherLibraryID); err != nil || named {
		t.Fatalf("a pin of a deleted Project's Deployment: %v, %v", named, err)
	}
}

// TestRollback_AppendsADeploymentWithTheCopiedDocument: a Rollback copies the
// document of an earlier Deployment into a new number, records the cause and the
// number it copied, and leaves the history as it was (ADR 0014).
func TestRollback_AppendsADeploymentWithTheCopiedDocument(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	states := runtimepg.NewTagStateRepositoryPostgres(db)
	ctx := context.Background()

	keptTag, addedTag := uuid.New(), uuid.New()
	first := documentOf(projectID, withTags(projectv0.TagDefinition{ID: keptTag, Name: "speed", Type: "integer"}))
	start := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	mustCreateDeployment(t, repo, projectID.UUID(), first, start)
	writeValue(t, states, projectID, keptTag, 42)

	second := documentOf(projectID, withTags(
		projectv0.TagDefinition{ID: keptTag, Name: "speed", Type: "integer"},
		projectv0.TagDefinition{ID: addedTag, Name: "flow", Type: "integer"},
	))
	mustCreateDeployment(t, repo, projectID.UUID(), second, start.Add(time.Minute))

	rolled, err := repo.Rollback(ctx, projectID.UUID(), 1, start.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if rolled.Number != 3 {
		t.Errorf("the rolled-back deployment is %d, want 3", rolled.Number)
	}
	if rolled.Cause != runtimeapp.DeploymentCauseRollback {
		t.Errorf("the cause is %q, want rollback", rolled.Cause)
	}
	if rolled.RolledBackFrom == nil || *rolled.RolledBackFrom != 1 {
		t.Errorf("rolled_back_from is %v, want 1", rolled.RolledBackFrom)
	}
	if !reflect.DeepEqual(rolled.Content, first) {
		t.Errorf("the rolled-back document %+v, want the document of deployment 1", rolled.Content)
	}

	var cause string
	var rolledBackFrom *int
	if err := db.QueryRowContext(ctx,
		`SELECT cause, rolled_back_from FROM runtime.deployments WHERE project_id = $1 AND number = 3`,
		projectID.UUID(),
	).Scan(&cause, &rolledBackFrom); err != nil {
		t.Fatalf("read the stored deployment: %v", err)
	}
	if cause != "rollback" || rolledBackFrom == nil || *rolledBackFrom != 1 {
		t.Errorf("the stored deployment is %q from %v, want rollback from 1", cause, rolledBackFrom)
	}

	// The copies follow the copied document: the Tag it does not have goes
	// with its state, the one it has keeps its value.
	if got := countRows(t, `SELECT count(*) FROM runtime.tags WHERE id = $1`, addedTag); got != 0 {
		t.Errorf("the copy of the tag the copied deployment lacks is still stored")
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.tag_states WHERE tag_id = $1`, addedTag); got != 0 {
		t.Errorf("the state of the tag the copied deployment lacks is still stored")
	}
	if got := findValue(t, states, projectID, keptTag); got != "42" {
		t.Errorf("the state of the kept tag is %q, want 42", got)
	}

	// History is not rewritten: deployment 1 is still there, intact.
	firstCopy, err := repo.FindByNumber(ctx, projectID.UUID(), 1)
	if err != nil {
		t.Fatalf("FindByNumber 1: %v", err)
	}
	if firstCopy.Cause != runtimeapp.DeploymentCauseDeploy || firstCopy.RolledBackFrom != nil {
		t.Errorf("deployment 1 is %q from %v, want a deploy", firstCopy.Cause, firstCopy.RolledBackFrom)
	}
	if !reflect.DeepEqual(firstCopy.Content, first) {
		t.Errorf("deployment 1 changed: %+v", firstCopy.Content)
	}
	if current, ok := mustFindCurrent(t, repo, projectID.UUID()); !ok || current.Number != 3 {
		t.Errorf("the running deployment is %d (%v), want 3", current.Number, ok)
	}
}

// TestRollback_KeepsTokensAndResetsAChangedTagType: the copies follow the
// copied document through the same diff a Deploy runs — a Tag whose TagType
// changed since the target Deployment starts from the target type's zero value,
// and a Device Token outlives the Device the target document does not have
// (task 36, Q6).
func TestRollback_KeepsTokensAndResetsAChangedTagType(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	states := runtimepg.NewTagStateRepositoryPostgres(db)
	ctx := context.Background()

	retyped, kept := uuid.New(), uuid.New()
	deviceID := uuid.New()
	first := documentOf(projectID,
		withTags(
			projectv0.TagDefinition{ID: retyped, Name: "counter", Type: "integer"},
			projectv0.TagDefinition{ID: kept, Name: "plain", Type: "integer"},
		),
	)
	mustCreateDeployment(t, repo, projectID.UUID(), first, time.Now())
	writeValue(t, states, projectID, retyped, 42)
	writeValue(t, states, projectID, kept, 7)

	second := documentOf(projectID,
		withTags(
			projectv0.TagDefinition{ID: retyped, Name: "counter", Type: "string"},
			projectv0.TagDefinition{ID: kept, Name: "plain", Type: "integer"},
		),
		withDevices(projectv0.Device{ID: deviceID, Name: "meter", Protocol: "simulation", Config: projectv0.JSONObject(`{"host": "meter"}`)}),
	)
	mustCreateDeployment(t, repo, projectID.UUID(), second, time.Now())
	if _, err := db.ExecContext(ctx,
		`INSERT INTO runtime.device_tokens (device_id, project_id, token_hash, fingerprint, issued_at)
		 VALUES ($1, $2, $3, 'gsc_dev_…', now())`,
		deviceID, projectID.UUID(), uuid.NewString(),
	); err != nil {
		t.Fatalf("insert token: %v", err)
	}

	if _, err := repo.Rollback(ctx, projectID.UUID(), 1, time.Now()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	state, ok := stateOf(t, states, projectID, retyped)
	if !ok {
		t.Fatal("the state of the retyped tag is gone")
	}
	if state.Value().String() != "0" || state.Quality().String() != "bad" {
		t.Errorf("the retyped tag is %q/%s, want the initial state of an integer tag", state.Value().String(), state.Quality().String())
	}
	if got := findValue(t, states, projectID, kept); got != "7" {
		t.Errorf("the state of the unchanged tag is %q, want 7", got)
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.devices WHERE id = $1`, deviceID); got != 0 {
		t.Errorf("the copy of the device the copied deployment lacks is still stored")
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.device_tokens WHERE device_id = $1`, deviceID); got != 1 {
		t.Errorf("the token of the dropped device is gone: a token outlives a rollback")
	}
}

// TestRollback_OfTheRunningContent_IsRefused: the numbers differ, the
// documents do not, and no Deployment is appended (ADR 0014).
func TestRollback_OfTheRunningContent_IsRefused(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	file := documentOf(projectID, withTags(projectv0.TagDefinition{ID: uuid.New(), Name: "speed", Type: "integer"}))
	mustCreateDeployment(t, repo, projectID.UUID(), file, time.Now())

	if _, err := repo.Rollback(ctx, projectID.UUID(), 1, time.Now()); !errors.Is(err, runtimeapp.ErrNothingToRollback) {
		t.Fatalf("Rollback to the running deployment: %v, want ErrNothingToRollback", err)
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.deployments WHERE project_id = $1`, projectID.UUID()); got != 1 {
		t.Errorf("stored deployments %d, want 1", got)
	}
}

// TestRollback_OfADeploymentTheProjectDoesNotHave: a number the Project has no
// Deployment for, a number of another Project, and a Project without any
// Deployment are all a not-found.
func TestRollback_OfADeploymentTheProjectDoesNotHave(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	if _, err := repo.Rollback(ctx, projectID.UUID(), 1, time.Now()); !errors.Is(err, runtimeapp.ErrDeploymentNotFound) {
		t.Errorf("Rollback of a project without deployments: %v, want ErrDeploymentNotFound", err)
	}
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID), time.Now())
	if _, err := repo.Rollback(ctx, projectID.UUID(), 7, time.Now()); !errors.Is(err, runtimeapp.ErrDeploymentNotFound) {
		t.Errorf("Rollback of a missing number: %v, want ErrDeploymentNotFound", err)
	}
	if _, err := repo.FindByNumber(ctx, projectID.UUID(), 7); !errors.Is(err, runtimeapp.ErrDeploymentNotFound) {
		t.Errorf("FindByNumber of a missing number: %v, want ErrDeploymentNotFound", err)
	}

	otherID := id.NewID[project.Project]()
	if _, err := repo.Rollback(ctx, otherID.UUID(), 1, time.Now()); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("Rollback of a missing project: %v, want ErrProjectNotFound", err)
	}

	// A number another Project has is not this Project's.
	anotherID, _ := newProject(t)
	mustCreateDeployment(t, repo, anotherID.UUID(), documentOf(anotherID), time.Now())
	mustCreateDeployment(t, repo, anotherID.UUID(), documentOf(anotherID), time.Now())
	if _, err := repo.Rollback(ctx, projectID.UUID(), 2, time.Now()); !errors.Is(err, runtimeapp.ErrDeploymentNotFound) {
		t.Errorf("Rollback to a number only another project has: %v, want ErrDeploymentNotFound", err)
	}
	if _, err := repo.FindByNumber(ctx, projectID.UUID(), 2); !errors.Is(err, runtimeapp.ErrDeploymentNotFound) {
		t.Errorf("FindByNumber of a number only another project has: %v, want ErrDeploymentNotFound", err)
	}
}

// TestDeploymentReads_OfADeletedProject_AreANotFound: the rows of a deleted
// Project are not its Deployments to serve until its cleanup removes them.
func TestDeploymentReads_OfADeletedProject_AreANotFound(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	ctx := context.Background()
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID), time.Now())

	if _, err := db.ExecContext(ctx, `UPDATE projects SET deleted_at = now() WHERE id = $1`, projectID.UUID()); err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	if _, err := repo.FindAll(ctx, projectID.UUID()); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("FindAll of a deleted project: %v, want ErrProjectNotFound", err)
	}
	if _, err := repo.FindByNumber(ctx, projectID.UUID(), 1); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("FindByNumber of a deleted project: %v, want ErrProjectNotFound", err)
	}
	if _, err := repo.Rollback(ctx, projectID.UUID(), 1, time.Now()); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("Rollback of a deleted project: %v, want ErrProjectNotFound", err)
	}
}

// TestRollback_AndADeploy_Serialize: one writer takes the advisory lock and
// the other waits, so the numbers never collide.
func TestRollback_AndADeploy_Serialize(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withTags(projectv0.TagDefinition{ID: uuid.New(), Name: "first", Type: "integer"})), time.Now())
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withTags(projectv0.TagDefinition{ID: uuid.New(), Name: "second", Type: "integer"})), time.Now())

	numbers := make(chan int, 2)
	errs := make(chan error, 2)
	go func() {
		deployment, err := repo.Rollback(ctx, projectID.UUID(), 1, time.Now())
		numbers <- deployment.Number
		errs <- err
	}()
	go func() {
		deployment, err := repo.CreateDeployment(ctx, projectID.UUID(), documentOf(projectID,
			withTags(projectv0.TagDefinition{ID: uuid.New(), Name: "third", Type: "integer"})), runtimeapp.DeploymentCauseDeploy, nil, time.Now())
		numbers <- deployment.Number
		errs <- err
	}()
	got := []int{}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent writer: %v", err)
		}
		got = append(got, <-numbers)
	}
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{3, 4}) {
		t.Errorf("the two writers took numbers %v, want 3 and 4", got)
	}
	if n := countRows(t, `SELECT count(*) FROM runtime.deployments WHERE project_id = $1`, projectID.UUID()); n != 4 {
		t.Errorf("stored deployments %d, want 4", n)
	}
}

// TestRollback_TwoConcurrent_Serialize: two Rollbacks take the advisory lock
// in turn, so both append a Deployment and the numbers never collide.
func TestRollback_TwoConcurrent_Serialize(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	for i, name := range []string{"first", "second", "third"} {
		mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
			withTags(projectv0.TagDefinition{ID: uuid.New(), Name: name, Type: "integer"})),
			time.Now().Add(time.Duration(i)*time.Minute))
	}

	numbers := make(chan int, 2)
	errs := make(chan error, 2)
	for _, target := range []int{1, 2} {
		go func() {
			deployment, err := repo.Rollback(ctx, projectID.UUID(), target, time.Now())
			numbers <- deployment.Number
			errs <- err
		}()
	}
	got := []int{}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent rollback: %v", err)
		}
		got = append(got, <-numbers)
	}
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{4, 5}) {
		t.Errorf("the two rollbacks took numbers %v, want 4 and 5", got)
	}
	if n := countRows(t, `SELECT count(*) FROM runtime.deployments WHERE project_id = $1`, projectID.UUID()); n != 5 {
		t.Errorf("stored deployments %d, want 5", n)
	}
}

// TestFindAll_IsNewestFirstAndWithoutDocuments: the list carries the summaries
// only, and a Project without a Deployment has an empty one.
func TestFindAll_IsNewestFirstAndWithoutDocuments(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	empty, err := repo.FindAll(ctx, projectID.UUID())
	if err != nil {
		t.Fatalf("FindAll of a project without deployments: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("FindAll of a project without deployments = %+v, want an empty list", empty)
	}

	tagID := uuid.New()
	start := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withTags(projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer"})), start)
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID,
		withTags(
			projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer"},
			projectv0.TagDefinition{ID: uuid.New(), Name: "flow", Type: "integer"},
		)), start.Add(time.Minute))
	if _, err := repo.Rollback(ctx, projectID.UUID(), 1, start.Add(2*time.Minute)); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	summaries, err := repo.FindAll(ctx, projectID.UUID())
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(summaries) != 3 {
		t.Fatalf("FindAll returned %d summaries, want 3", len(summaries))
	}
	for i, want := range []struct {
		number int
		cause  runtimeapp.DeploymentCause
		from   *int
	}{
		{3, runtimeapp.DeploymentCauseRollback, ptr(1)},
		{2, runtimeapp.DeploymentCauseDeploy, nil},
		{1, runtimeapp.DeploymentCauseDeploy, nil},
	} {
		got := summaries[i]
		if got.Number != want.number || got.Cause != want.cause {
			t.Errorf("summary %d = %+v, want #%d %s", i, got, want.number, want.cause)
		}
		if (got.RolledBackFrom == nil) != (want.from == nil) || (want.from != nil && *got.RolledBackFrom != *want.from) {
			t.Errorf("summary %d rolled back from %v, want %v", i, got.RolledBackFrom, want.from)
		}
		if got.DeployedAt.IsZero() {
			t.Errorf("summary %d has no time", i)
		}
	}
}

func ptr(n int) *int { return &n }

// TestDeploymentCause_Constraints: the store refuses a cause it does not know
// and a rollback that does not name the Deployment it copied.
func TestDeploymentCause_Constraints(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(db)
	mustCreateDeployment(t, repo, projectID.UUID(), documentOf(projectID), time.Now())

	for name, insert := range map[string]struct {
		cause string
		from  *int
	}{
		"unknown cause":         {"reset", nil},
		"rollback without":      {"rollback", nil},
		"deploy with one":       {"deploy", ptr(1)},
		"rollback from nothing": {"rollback", ptr(9)},
	} {
		_, err := db.ExecContext(context.Background(),
			`INSERT INTO runtime.deployments (project_id, number, deployed_at, cause, rolled_back_from, config)
			 VALUES ($1, 2, now(), $2, $3, '{}')`,
			projectID.UUID(), insert.cause, insert.from,
		)
		if err == nil {
			t.Errorf("%s: the row was taken", name)
			_, _ = db.ExecContext(context.Background(),
				`DELETE FROM runtime.deployments WHERE project_id = $1 AND number = 2`, projectID.UUID())
		}
	}
}

// writeValue stores an integer value with Quality good in the state of the
// Tag.
func writeValue(t *testing.T, states tagstate.Repository, projectID id.ID[project.Project], tagID uuid.UUID, value int) {
	t.Helper()
	state, ok := stateOf(t, states, projectID, tagID)
	if !ok {
		t.Fatalf("no state of tag %s", tagID)
	}
	written, err := state.Write(value, tag.TagQualityGood)
	if err != nil {
		t.Fatalf("write value %d to tag %s: %v", value, tagID, err)
	}
	if _, _, err := states.Write(context.Background(), written, tag.NoSupplier()); err != nil {
		t.Fatalf("store value of tag %s: %v", tagID, err)
	}
}

func stateOf(t *testing.T, states tagstate.Repository, projectID id.ID[project.Project], tagID uuid.UUID) (tagstate.TagState, bool) {
	t.Helper()
	found, err := states.FindByIDs(context.Background(), projectID, []id.ID[tag.Tag]{id.NewID(id.IDWithUUID[tag.Tag](tagID))})
	if err != nil {
		t.Fatalf("FindByIDs: %v", err)
	}
	state, ok := found[id.NewID(id.IDWithUUID[tag.Tag](tagID))]
	return state, ok
}

func findValue(t *testing.T, states tagstate.Repository, projectID id.ID[project.Project], tagID uuid.UUID) string {
	t.Helper()
	state, ok := stateOf(t, states, projectID, tagID)
	if !ok {
		t.Fatalf("no state of tag %s", tagID)
	}
	return state.Value().String()
}

// TestCreateDeployment_ReleaseNumber: a Deploy of a Release stores and serves
// its number, refuses a second Deploy of the same Release, and a Deployment
// from a file states none (task 72_4).
func TestCreateDeployment_ReleaseNumber(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	first := 1

	deployment, err := repo.CreateDeployment(ctx, projectID.UUID(), documentOf(projectID),
		runtimeapp.DeploymentCauseDeploy, &first, time.Now())
	if err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	if deployment.ReleaseNumber == nil || *deployment.ReleaseNumber != 1 {
		t.Errorf("the new deployment names release %v, want 1", deployment.ReleaseNumber)
	}
	current, ok, err := repo.FindCurrent(ctx, projectID.UUID())
	if err != nil || !ok || current.ReleaseNumber == nil || *current.ReleaseNumber != 1 {
		t.Errorf("FindCurrent: %+v, %t, %v, want release 1", current, ok, err)
	}
	summaries, err := repo.FindAll(ctx, projectID.UUID())
	if err != nil || len(summaries) != 1 || summaries[0].ReleaseNumber == nil || *summaries[0].ReleaseNumber != 1 {
		t.Errorf("FindAll: %+v, %v, want release 1", summaries, err)
	}
	found, err := repo.FindByNumber(ctx, projectID.UUID(), deployment.Number)
	if err != nil || found.ReleaseNumber == nil || *found.ReleaseNumber != 1 {
		t.Errorf("FindByNumber: %+v, %v, want release 1", found, err)
	}

	if _, err := repo.CreateDeployment(ctx, projectID.UUID(), documentOf(projectID),
		runtimeapp.DeploymentCauseDeploy, &first, time.Now()); !errors.Is(err, runtimeapp.ErrNothingToDeploy) {
		t.Errorf("a second Deploy of release 1: %v, want ErrNothingToDeploy", err)
	}
	if _, err := repo.CreateDeployment(ctx, projectID.UUID(), documentOf(projectID),
		runtimeapp.DeploymentCauseFile, nil, time.Now()); err != nil {
		t.Fatalf("a file deployment: %v", err)
	}
	fileCurrent, _, err := repo.FindCurrent(ctx, projectID.UUID())
	if err != nil || fileCurrent.ReleaseNumber != nil {
		t.Errorf("a file deployment names release %v, want none: %v", fileCurrent.ReleaseNumber, err)
	}
}

// TestCreateDeployment_TwoRacingDeploysOfOneRelease: the check of the running
// Release shares the advisory lock with the numbering, so two Deploys of one
// Release race into one Deployment and one refusal (task 72_4).
func TestCreateDeployment_TwoRacingDeploysOfOneRelease(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	first := 1

	var (
		wg                sync.WaitGroup
		mu                sync.Mutex
		deployed, refused int
	)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.CreateDeployment(ctx, projectID.UUID(), documentOf(projectID),
				runtimeapp.DeploymentCauseDeploy, &first, time.Now())
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				deployed++
			case errors.Is(err, runtimeapp.ErrNothingToDeploy):
				refused++
			default:
				t.Errorf("Deploy: %v, want no error or ErrNothingToDeploy", err)
			}
		}()
	}
	wg.Wait()

	if deployed != 1 || refused != 1 {
		t.Fatalf("%d Deploys and %d refusals, want one of each", deployed, refused)
	}
	summaries, err := repo.FindAll(ctx, projectID.UUID())
	if err != nil || len(summaries) != 1 {
		t.Errorf("the Deployments after the race: %+v, %v, want one", summaries, err)
	}
}

// TestRollback_KeepsTheReleaseNumber: a Rollback copies the Release of the
// Deployment it names (task 72_4).
func TestRollback_KeepsTheReleaseNumber(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewDeploymentRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()
	firstRelease, secondRelease := 1, 2

	first, err := repo.CreateDeployment(ctx, projectID.UUID(),
		documentOf(projectID, withTags(documentTag(definition(t, projectID, "speed", tag.TagTypeInteger)))),
		runtimeapp.DeploymentCauseDeploy, &firstRelease, time.Now())
	if err != nil {
		t.Fatalf("CreateDeployment 1: %v", err)
	}
	if _, err := repo.CreateDeployment(ctx, projectID.UUID(),
		documentOf(projectID, withTags(documentTag(definition(t, projectID, "flow", tag.TagTypeInteger)))),
		runtimeapp.DeploymentCauseDeploy, &secondRelease, time.Now()); err != nil {
		t.Fatalf("CreateDeployment 2: %v", err)
	}

	rolled, err := repo.Rollback(ctx, projectID.UUID(), first.Number, time.Now())
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if rolled.ReleaseNumber == nil || *rolled.ReleaseNumber != 1 {
		t.Errorf("the rollback names release %v, want 1", rolled.ReleaseNumber)
	}
}
