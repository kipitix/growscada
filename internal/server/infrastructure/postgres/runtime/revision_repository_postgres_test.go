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

func mustCreateRevision(t *testing.T, repo runtimeapp.RevisionRepository, projectID uuid.UUID, file projectv0.ProjectFile, at time.Time) runtimeapp.Revision {
	t.Helper()
	revision, err := repo.CreateRevision(context.Background(), projectID, file, at)
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	return revision
}

func mustFindCurrent(t *testing.T, repo runtimeapp.RevisionRepository, projectID uuid.UUID) (runtimeapp.Revision, bool) {
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

func TestCreateRevision_NumbersRevisionsAndKeepsTheDocument(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewRevisionRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	if _, ok := mustFindCurrent(t, repo, projectID.UUID()); ok {
		t.Fatal("a project that was never deployed has a revision")
	}

	tagID := uuid.New()
	first := documentOf(projectID, withTags(projectv0.TagDefinition{ID: tagID, Name: "speed", Type: "integer"}))
	start := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		file := first
		file.Tags = []projectv0.TagDefinition{{ID: tagID, Name: "speed", Type: "integer"}}
		at := start.Add(time.Duration(i) * time.Minute)
		revision, err := repo.CreateRevision(ctx, projectID.UUID(), file, at)
		if err != nil {
			t.Fatalf("CreateRevision %d: %v", i, err)
		}
		if revision.Number != i {
			t.Errorf("revision %d created with number %d", i, revision.Number)
		}
		if !revision.DeployedAt.Equal(at) {
			t.Errorf("revision %d deployed at %s, want %s", i, revision.DeployedAt, at)
		}
		if !reflect.DeepEqual(revision.Content, file) {
			t.Errorf("revision %d content differs from the document", i)
		}
	}

	current, ok := mustFindCurrent(t, repo, projectID.UUID())
	if !ok {
		t.Fatal("a deployed project has no revision")
	}
	if current.Number != 3 {
		t.Errorf("current revision %d, want 3", current.Number)
	}
	if !current.DeployedAt.Equal(start.Add(3 * time.Minute)) {
		t.Errorf("current revision deployed at %s", current.DeployedAt)
	}
	if !reflect.DeepEqual(current.Content.Tags, first.Tags) || current.Content.ProjectID != projectID.UUID() {
		t.Errorf("current revision content %+v, want %+v", current.Content, first)
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.revisions WHERE project_id = $1`, projectID.UUID()); got != 3 {
		t.Errorf("stored revisions %d, want 3", got)
	}
}

func TestCreateRevision_ConcurrentDeploysTakeDistinctNumbers(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewRevisionRepositoryPostgres(postgres.DB(t))
	ctx := context.Background()

	const deploys = 8
	numbers := make([]int, deploys)
	errs := make([]error, deploys)
	var wg sync.WaitGroup
	for i := 0; i < deploys; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			revision, err := repo.CreateRevision(ctx, projectID.UUID(), documentOf(projectID), time.Now())
			numbers[i], errs[i] = revision.Number, err
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
	if got := countRows(t, `SELECT count(*) FROM runtime.revisions WHERE project_id = $1`, projectID.UUID()); got != deploys {
		t.Errorf("stored revisions %d, want %d", got, deploys)
	}
	if current, ok := mustFindCurrent(t, repo, projectID.UUID()); !ok || current.Number != deploys {
		t.Errorf("current revision %+v (%v), want %d", current.Number, ok, deploys)
	}
}

func TestCreateRevision_RefusesARevisionNumberTaken(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewRevisionRepositoryPostgres(postgres.DB(t))
	mustCreateRevision(t, repo, projectID.UUID(), documentOf(projectID), time.Now())

	_, err := postgres.DB(t).ExecContext(context.Background(),
		`INSERT INTO runtime.revisions (project_id, n, deployed_at, config) VALUES ($1, 1, now(), '{}')`,
		projectID.UUID(),
	)
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) || pqErr.Code != "23505" {
		t.Fatalf("a second revision 1: %v, want a unique violation", err)
	}
}

func TestCreateRevision_KeepsStatesAndTokensAndDropsAbsentCopies(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	ctx := context.Background()
	repo := runtimepg.NewRevisionRepositoryPostgres(db)
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
	mustCreateRevision(t, repo, projectID.UUID(), first, time.Now())

	writeValue(t, states, projectID, keptTag, 42)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO runtime.device_tokens (device_id, project_id, token_hash, fingerprint, issued_at)
		 VALUES ($1, $2, 'hash', 'gsc_dev_…', now())`,
		deviceID, projectID.UUID(),
	); err != nil {
		t.Fatalf("insert token: %v", err)
	}

	second := documentOf(projectID, withTags(projectv0.TagDefinition{ID: keptTag, Name: "kept", Type: "integer"}))
	mustCreateRevision(t, repo, projectID.UUID(), second, time.Now())

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
	mustCreateRevision(t, repo, projectID.UUID(), third, time.Now())

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
		t.Errorf("the pair params are %s, want the address of the new revision", params)
	}
}

func TestCreateRevision_AnEmptyConfigAndParamsAreTheEmptyObject(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewRevisionRepositoryPostgres(db)
	deviceID, environmentID := uuid.New(), uuid.New()

	mustCreateRevision(t, repo, projectID.UUID(), documentOf(projectID,
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

func TestCreateRevision_RefusesAPairWhoseCopiesAreOfAnotherProject(t *testing.T) {
	projectID, orgID := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewRevisionRepositoryPostgres(db)
	ctx := context.Background()

	otherID := id.NewID[project.Project]()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO projects (id, organization_id, name, version) VALUES ($1, $2, $3, 1)`,
		otherID.UUID(), orgID.UUID(), "p-"+otherID.String()); err != nil {
		t.Fatalf("insert the other project: %v", err)
	}
	deviceID, environmentID := uuid.New(), uuid.New()
	mustCreateRevision(t, repo, otherID.UUID(), documentOf(otherID,
		withDevices(projectv0.Device{ID: deviceID, Name: "meter", Protocol: "simulation", Config: projectv0.JSONObject(`{"host": "meter"}`)}),
		withEnvironments(projectv0.Environment{ID: environmentID, Name: "bench"}),
		withEnvironmentDevices(projectv0.EnvironmentDevice{
			EnvironmentID: environmentID, DeviceID: deviceID,
			Params: projectv0.JSONObject(`{"address": "10.0.0.1"}`),
		}),
	), time.Now())

	_, err := repo.CreateRevision(ctx, projectID.UUID(), documentOf(projectID,
		withEnvironmentDevices(projectv0.EnvironmentDevice{
			EnvironmentID: environmentID, DeviceID: deviceID,
			Params: projectv0.JSONObject(`{"address": "10.9.9.9"}`),
		}),
	), time.Now())
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

func TestCreateRevision_HealsACopyThatLostItsState(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewRevisionRepositoryPostgres(db)
	states := runtimepg.NewTagStateRepositoryPostgres(db)
	tagID := uuid.New()
	file := documentOf(projectID, withTags(projectv0.TagDefinition{ID: tagID, Name: "counter", Type: "integer"}))

	mustCreateRevision(t, repo, projectID.UUID(), file, time.Now())
	if _, err := db.ExecContext(context.Background(), `DELETE FROM runtime.tag_states WHERE tag_id = $1`, tagID); err != nil {
		t.Fatalf("drop the state: %v", err)
	}
	if _, ok := stateOf(t, states, projectID, tagID); ok {
		t.Fatal("the state survives its deletion")
	}

	mustCreateRevision(t, repo, projectID.UUID(), file, time.Now())
	state, ok := stateOf(t, states, projectID, tagID)
	if !ok {
		t.Fatal("the deploy of an unchanged document did not give the copy its state back")
	}
	if state.Value().String() != "0" || state.Quality().String() != "bad" {
		t.Errorf("the healed state is %q/%s, want the initial state of an integer tag", state.Value().String(), state.Quality().String())
	}
}

func TestCreateRevision_TwoEnvironmentsMaySwapTheirNames(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewRevisionRepositoryPostgres(db)
	ctx := context.Background()
	first, second := uuid.New(), uuid.New()

	mustCreateRevision(t, repo, projectID.UUID(), documentOf(projectID,
		withEnvironments(
			projectv0.Environment{ID: first, Name: "bench"},
			projectv0.Environment{ID: second, Name: "lab"},
		),
	), time.Now())

	// The first write takes the name the second one has not left yet.
	mustCreateRevision(t, repo, projectID.UUID(), documentOf(projectID,
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

func TestCreateRevision_RefusesTwoEnvironmentsOfOneName(t *testing.T) {
	projectID, _ := newProject(t)
	repo := runtimepg.NewRevisionRepositoryPostgres(postgres.DB(t))

	_, err := repo.CreateRevision(context.Background(), projectID.UUID(), documentOf(projectID,
		withEnvironments(
			projectv0.Environment{ID: uuid.New(), Name: "bench"},
			projectv0.Environment{ID: uuid.New(), Name: "bench"},
		),
	), time.Now())
	if err == nil {
		t.Fatal("two environments of one name were taken")
	}
}

func TestCreateRevision_ResetsTheStateOfAChangedTagTypeAndKeepsTheOthers(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewRevisionRepositoryPostgres(db)
	states := runtimepg.NewTagStateRepositoryPostgres(db)

	changed, renamed := uuid.New(), uuid.New()
	mustCreateRevision(t, repo, projectID.UUID(), documentOf(projectID,
		withTags(
			projectv0.TagDefinition{ID: changed, Name: "counter", Type: "integer"},
			projectv0.TagDefinition{ID: renamed, Name: "plain", Type: "integer"},
		),
	), time.Now())
	writeValue(t, states, projectID, changed, 42)
	writeValue(t, states, projectID, renamed, 7)

	mustCreateRevision(t, repo, projectID.UUID(), documentOf(projectID,
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

func TestCreateRevision_TheProjectTakesItsRevisionsAlong(t *testing.T) {
	projectID, _ := newProject(t)
	db := postgres.DB(t)
	repo := runtimepg.NewRevisionRepositoryPostgres(db)
	mustCreateRevision(t, repo, projectID.UUID(), documentOf(projectID), time.Now())

	if _, err := db.ExecContext(context.Background(), `DELETE FROM projects WHERE id = $1`, projectID.UUID()); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if got := countRows(t, `SELECT count(*) FROM runtime.revisions WHERE project_id = $1`, projectID.UUID()); got != 0 {
		t.Errorf("revisions of the deleted project: %d, want none", got)
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
	if _, err := states.Write(context.Background(), written, tag.NoSupplier()); err != nil {
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
