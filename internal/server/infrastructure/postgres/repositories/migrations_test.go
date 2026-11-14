package repositories_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"

	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/migrations"
)

// migrationDB creates an empty database of its own on the test server, so
// that a migration can be run against data written in an older schema.
func migrationDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	name := "migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := testDB.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	db, err := sql.Open("postgres", strings.Replace(testConnStr, "/testdb?", "/"+name+"?", 1))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		_, _ = testDB.Exec("DROP DATABASE IF EXISTS " + name)
	})
	_, currentFile, _, _ := runtime.Caller(0)
	return db, filepath.Join(filepath.Dir(currentFile), "..", "migrations", "public")
}

// rolesBaseline is the version the public migration marked both roles' chains
// at: the public chain goes back only from it, so the roles' chains are rolled
// back to it first (ADR 0011).
const rolesBaseline = 20261026000000

// rollRolesBackToBaseline rolls the chain of each role back to rolesBaseline,
// which the public chain's Down requires. A test that expects the public chain
// to go back calls it once the roles' tables have the data it checks.
func rollRolesBackToBaseline(t *testing.T, ctx context.Context, db *sql.DB, dir string) {
	t.Helper()
	for _, c := range []struct {
		dir   string
		table string
	}{
		{filepath.Join(dir, "..", "runtime"), "runtime.goose_db_version"},
		{filepath.Join(dir, "..", "engineering"), "engineering.goose_db_version"},
	} {
		provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(c.dir), goose.WithTableName(c.table))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.DownTo(ctx, rolesBaseline); err != nil {
			t.Fatalf("roll %s back to its baseline: %v", c.table, err)
		}
	}
}

func TestMigrationWidgetsTypeIDForeignKey_BumpsVersionOfScenesThatLoseWidgets(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	if err := goose.UpToContext(ctx, db, dir, 20260926000000); err != nil {
		t.Fatalf("migrate to the previous version: %v", err)
	}

	typeID, orphanScene, keptScene := uuid.New(), uuid.New(), uuid.New()
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	mustExec(`INSERT INTO widget_types (id, name, html_template, script, script_language)
	          VALUES ($1, 'kept type', '', '', 'javascript')`, typeID)
	mustExec(`INSERT INTO scenes (id, name, width, height, background_html, version)
	          VALUES ($1, 'orphan scene', 100, 100, '', 3), ($2, 'kept scene', 100, 100, '', 7)`,
		orphanScene, keptScene)
	mustExec(`INSERT INTO widgets (id, name, type_id, scene_id)
	          VALUES ($1, 'orphan', $2, $3), ($4, 'kept', $5, $6)`,
		uuid.New(), uuid.New(), orphanScene, uuid.New(), typeID, keptScene)

	if err := goose.UpToContext(ctx, db, dir, 20261004000000); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	versions := map[uuid.UUID]int{}
	rows, err := db.QueryContext(ctx, `SELECT id, version FROM scenes`)
	if err != nil {
		t.Fatalf("select scenes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var v int
		if err := rows.Scan(&id, &v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		versions[id] = v
	}
	if versions[orphanScene] != 4 {
		t.Errorf("scene that lost a widget: expected version 4, got %d", versions[orphanScene])
	}
	if versions[keptScene] != 7 {
		t.Errorf("scene that kept its widgets: expected version 7, got %d", versions[keptScene])
	}
}

func TestMigrationOrganizationsAndProjects_PutsExistingContentIntoMain(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	if err := goose.UpToContext(ctx, db, dir, 20261019000001); err != nil {
		t.Fatalf("migrate to the previous version: %v", err)
	}
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	typeID, sceneID, tagID := uuid.New(), uuid.New(), uuid.New()
	mustExec(`INSERT INTO widget_types (id, name, html_template, script, script_language)
	          VALUES ($1, 'type', '', '', 'javascript')`, typeID)
	mustExec(`INSERT INTO scenes (id, name) VALUES ($1, 'scene')`, sceneID)
	mustExec(`INSERT INTO widgets (id, name, type_id, scene_id) VALUES ($1, 'w', $2, $3)`, uuid.New(), typeID, sceneID)
	mustExec(`INSERT INTO tags (id, name, type, value, quality) VALUES ($1, 'speed', 'integer', '0', 'good')`, tagID)
	mustExec(`INSERT INTO outbox (type, payload) VALUES ('tag_created', $1), ('widget_type_updated', $2)`,
		`{"timestamp":"2026-10-06T12:30:15Z","id":"`+tagID.String()+`"}`,
		`{"timestamp":"2026-10-06T12:30:15Z","id":"`+typeID.String()+`"}`)
	mustExec(`INSERT INTO policy_jobs (policy, job_key) VALUES
	          ('unbind_deleted_tag', 'tag:' || $1::text),
	          ('reconcile_widgets_with_widget_type', 'widget_type:' || $2::text || '/scene:' || $3::text),
	          ('reconcile_widgets_with_widget_type', 'widget_type:' || $2::text)`, tagID, typeID, sceneID)

	if err := goose.UpToContext(ctx, db, dir, 20261025000000); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const main = "b0000000-0000-4000-8000-000000000001"
	for _, table := range []string{"tags", "scenes", "widgets"} {
		var others int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table+` WHERE project_id <> $1`, main).Scan(&others); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if others != 0 {
			t.Errorf("%s: %d rows outside the project Main", table, others)
		}
	}
	var name string
	if err := db.QueryRowContext(ctx, `SELECT name FROM projects WHERE id = $1 AND deleted_at IS NULL`, main).Scan(&name); err != nil || name != "Main" {
		t.Errorf("project Main: %q, %v", name, err)
	}
	rows, err := db.QueryContext(ctx, `SELECT type, payload->>'project_id' FROM outbox ORDER BY seq`)
	if err != nil {
		t.Fatalf("outbox: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var eventType string
		var payload sql.NullString
		if err := rows.Scan(&eventType, &payload); err != nil {
			t.Fatal(err)
		}
		if wantProject := eventType == "tag_created"; (payload.String == main) != wantProject {
			t.Errorf("outbox %s: payload project_id %v", eventType, payload)
		}
	}
	keys := map[string]bool{}
	jobs, err := db.QueryContext(ctx, `SELECT job_key FROM policy_jobs`)
	if err != nil {
		t.Fatalf("policy_jobs: %v", err)
	}
	defer jobs.Close()
	for jobs.Next() {
		var key string
		if err := jobs.Scan(&key); err != nil {
			t.Fatal(err)
		}
		keys[key] = true
	}
	for _, want := range []string{
		"project:" + main + "/tag:" + tagID.String(),
		"widget_type:" + typeID.String() + "/project:" + main + "/scene:" + sceneID.String(),
		"widget_type:" + typeID.String(),
	} {
		if !keys[want] {
			t.Errorf("no job %q among %v", want, keys)
		}
	}
}

// TestMigrationOrganizationsAndProjects_Down_LeavesOutDeletedProjects: the
// content of a project deleted but not cleaned up yet does not come back
// merged into the one set, nor do its jobs or the events of projects.
func TestMigrationOrganizationsAndProjects_Down_LeavesOutDeletedProjects(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	if err := goose.UpToContext(ctx, db, dir, 20261025000000); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	const main = "b0000000-0000-4000-8000-000000000001"
	deleted, typeID := uuid.New(), uuid.New()
	mainTag, mainScene, goneTag, goneScene := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mustExec(`INSERT INTO projects (id, organization_id, name, version, deleted_at)
	          SELECT $1, id, 'Line 1', 2, now() FROM organizations`, deleted)
	mustExec(`INSERT INTO widget_types (id, name, html_template, script, script_language)
	          VALUES ($1, 'type', '', '', 'javascript')`, typeID)
	mustExec(`INSERT INTO tags (id, project_id, name, type, value, quality)
	          VALUES ($1, $2, 'speed', 'integer', '0', 'good'), ($3, $4, 'level', 'integer', '0', 'good')`,
		mainTag, main, goneTag, deleted)
	mustExec(`INSERT INTO scenes (id, project_id, name) VALUES ($1, $2, 'main scene'), ($3, $4, 'gone scene')`,
		mainScene, main, goneScene, deleted)
	mustExec(`INSERT INTO widgets (id, name, type_id, scene_id, project_id)
	          VALUES ($1, 'kept', $2, $3, $4), ($5, 'gone', $2, $6, $7)`,
		uuid.New(), typeID, mainScene, main, uuid.New(), goneScene, deleted)
	mustExec(`INSERT INTO outbox (type, payload) VALUES ('project_deleted', $1), ('tag_created', $2)`,
		`{"timestamp":"2026-10-26T12:00:00Z","id":"`+deleted.String()+`"}`,
		`{"timestamp":"2026-10-26T12:00:00Z","id":"`+mainTag.String()+`","project_id":"`+main+`"}`)
	mustExec(`INSERT INTO policy_jobs (policy, job_key) VALUES
	          ('remove_deleted_project', 'project:' || $1::text),
	          ('unbind_deleted_tag', 'project:' || $1::text || '/tag:' || $2::text),
	          ('unbind_deleted_tag', 'project:' || $3::text || '/tag:' || $4::text),
	          ('reconcile_widgets_with_widget_type', 'widget_type:' || $5::text || '/project:' || $1::text || '/scene:' || $6::text),
	          ('reconcile_widgets_with_widget_type', 'widget_type:' || $5::text || '/project:' || $3::text || '/scene:' || $7::text)`,
		deleted, goneTag, main, mainTag, typeID, goneScene, mainScene)

	if err := goose.DownToContext(ctx, db, dir, 20261019000001); err != nil {
		t.Fatalf("migrate down: %v", err)
	}

	ids := func(query string) []string {
		t.Helper()
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		defer rows.Close()
		var got []string
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				t.Fatal(err)
			}
			got = append(got, v)
		}
		slices.Sort(got)
		return got
	}
	for query, want := range map[string][]string{
		`SELECT id::text FROM tags`:                                     {mainTag.String()},
		`SELECT id::text FROM scenes`:                                   {mainScene.String()},
		`SELECT name FROM widgets`:                                      {"kept"},
		`SELECT type FROM outbox`:                                       {"tag_created"},
		`SELECT payload::text FROM outbox WHERE payload ? 'project_id'`: nil,
		`SELECT job_key FROM policy_jobs`: {
			"tag:" + mainTag.String(),
			"widget_type:" + typeID.String() + "/scene:" + mainScene.String(),
		},
	} {
		if got := ids(query); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", query, got, want)
		}
	}
}

// TestSeedData_AppliesOnTheMigratedSchema keeps the seed of make db_up and of
// make bruno_test in step with the migrations: the demo content in the
// project Main, every Tag with its state, and the empty project Line 2.
func TestSeedData_AppliesOnTheMigratedSchema(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := goose.UpContext(ctx, db, filepath.Join(dir, "..", "..", "test_data")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	const main = "b0000000-0000-4000-8000-000000000001"
	var unmatched int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.tags d
	                                   FULL JOIN runtime.tags c ON c.id = d.id
	                                   FULL JOIN runtime.tag_states s ON s.tag_id = d.id
	                                   WHERE d.id IS NULL OR c.id IS NULL OR s.tag_id IS NULL
	                                     OR d.type <> c.type OR d.name <> c.name`).Scan(&unmatched); err != nil || unmatched != 0 {
		t.Errorf("tag definitions without a state or the other way round: %d, %v", unmatched, err)
	}
	for _, table := range []string{"engineering.tags", "runtime.tags", "engineering.scenes", "engineering.widgets"} {
		var inMain, all int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE project_id = $1), count(*) FROM `+table, main).Scan(&inMain, &all); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if all == 0 || inMain != all {
			t.Errorf("%s: %d of %d rows in the project Main, want all of some", table, inMain, all)
		}
	}
	var names []string
	rows, err := db.QueryContext(ctx, `SELECT name FROM projects WHERE deleted_at IS NULL ORDER BY pk_id`)
	if err != nil {
		t.Fatalf("projects: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if strings.Join(names, ",") != "Main,Line 2" {
		t.Errorf("projects %v, want Main and Line 2", names)
	}
	// Two libraries, each released once with its Draft as it is; Main pins
	// both, Line 2 Main; every Widget of a pinned type.
	var libraries, unreleased, pins, unpinnedWidgets int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.libraries`).Scan(&libraries); err != nil || libraries != 2 {
		t.Errorf("libraries: %d, %v; want 2", libraries, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.widget_types w WHERE NOT EXISTS
	          (SELECT 1 FROM engineering.released_widget_types r WHERE r.id = w.id AND r.name = w.name AND r.release_number = 1)`).Scan(&unreleased); err != nil || unreleased != 0 {
		t.Errorf("widget types not in release 1: %d, %v", unreleased, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.library_imports`).Scan(&pins); err != nil || pins != 3 {
		t.Errorf("pins: %d, %v; want 3", pins, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.widgets w WHERE NOT EXISTS
	          (SELECT 1 FROM engineering.library_imports li JOIN engineering.commits c ON c.id = li.commit_id
	           WHERE li.project_id = w.project_id
	             AND EXISTS (SELECT 1 FROM jsonb_array_elements(c.content -> 'widget_types') wt
	                         WHERE (wt ->> 'id')::uuid = w.type_id))`).Scan(&unpinnedWidgets); err != nil || unpinnedWidgets != 0 {
		t.Errorf("widgets of a type not pinned: %d, %v", unpinnedWidgets, err)
	}
}

// TestMigrationEngineeringAndRuntimeSchemas_MovesTablesAndSplitsTags: the
// tables of the Draft move to engineering, each Tag's value and Quality to
// runtime.tags with its definition's copy and a seq in the order the Tags
// were created, tag_updated leaves the outbox, and the chains of the two
// schemas start at their baseline (ADR 0011).
func TestMigrationEngineeringAndRuntimeSchemas_MovesTablesAndSplitsTags(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	if err := goose.UpToContext(ctx, db, dir, 20261025000000); err != nil {
		t.Fatalf("migrate to the previous version: %v", err)
	}
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	const main = "b0000000-0000-4000-8000-000000000001"
	typeID, sceneID, first, second := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mustExec(`INSERT INTO widget_types (id, name, html_template, script, script_language)
	          VALUES ($1, 'type', '', '', 'javascript')`, typeID)
	mustExec(`INSERT INTO scenes (id, project_id, name) VALUES ($1, $2, 'scene')`, sceneID, main)
	mustExec(`INSERT INTO widgets (id, project_id, name, type_id, scene_id) VALUES ($1, $2, 'w', $3, $4)`, uuid.New(), main, typeID, sceneID)
	mustExec(`INSERT INTO tags (id, project_id, name, type, value, quality, version)
	          VALUES ($1, $2, 'speed', 'integer', '42', 'uncertain', 7), ($3, $2, 'mode', 'string', 'auto', 'good', 1)`, first, main, second)
	mustExec(`INSERT INTO outbox (type, payload) VALUES ('tag_updated', $1), ('tag_created', $2)`,
		`{"timestamp":"2026-10-26T12:00:00Z","id":"`+first.String()+`","project_id":"`+main+`","tag":{"name":"speed","type":"integer","value":"42","quality":"uncertain","version":7}}`,
		`{"timestamp":"2026-10-26T12:00:00Z","id":"`+second.String()+`","project_id":"`+main+`"}`)

	if err := goose.UpToContext(ctx, db, dir, 20261026000000); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for table, want := range map[string]int{
		"engineering.widget_types": 1, "engineering.scenes": 1, "engineering.widgets": 1, "engineering.tags": 2, "runtime.tags": 2,
	} {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != want {
			t.Errorf("%s: %d rows, %v; want %d", table, n, err, want)
		}
	}
	var left int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = 'public'
	                                   AND tablename IN ('tags', 'scenes', 'widgets', 'widget_types')`).Scan(&left); err != nil || left != 0 {
		t.Errorf("tables of the roles left in public: %d, %v", left, err)
	}
	var stateColumns int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema = 'engineering'
	                                   AND table_name = 'tags' AND column_name IN ('value', 'quality')`).Scan(&stateColumns); err != nil || stateColumns != 0 {
		t.Errorf("state columns left in the Tag definitions: %d, %v", stateColumns, err)
	}
	type state struct {
		name, tagType, value, quality string
		version                       int
		seq                           int64
	}
	states := map[uuid.UUID]state{}
	rows, err := db.QueryContext(ctx, `SELECT id, name, type, value, quality, version, seq FROM runtime.tags`)
	if err != nil {
		t.Fatalf("runtime.tags: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tagID uuid.UUID
		var s state
		if err := rows.Scan(&tagID, &s.name, &s.tagType, &s.value, &s.quality, &s.version, &s.seq); err != nil {
			t.Fatal(err)
		}
		states[tagID] = s
	}
	if s := states[first]; s.name != "speed" || s.tagType != "integer" || s.value != "42" || s.quality != "uncertain" || s.version != 7 {
		t.Errorf("state of speed: %+v", s)
	}
	if s := states[second]; s.value != "auto" || s.quality != "good" || s.seq <= states[first].seq || states[first].seq <= 0 {
		t.Errorf("state of mode: %+v, after speed's seq %d", s, states[first].seq)
	}
	var outboxTypes []string
	typeRows, err := db.QueryContext(ctx, `SELECT type FROM outbox ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer typeRows.Close()
	for typeRows.Next() {
		var eventType string
		if err := typeRows.Scan(&eventType); err != nil {
			t.Fatal(err)
		}
		outboxTypes = append(outboxTypes, eventType)
	}
	if !slices.Equal(outboxTypes, []string{"tag_created"}) {
		t.Errorf("outbox %v, want tag_updated gone", outboxTypes)
	}

	// The chains of the roles start at the baseline the migration marked:
	// applying every chain changes nothing more.
	for _, table := range []string{"engineering.goose_db_version", "runtime.goose_db_version"} {
		var latest int64
		if err := db.QueryRowContext(ctx, `SELECT max(version_id) FROM `+table).Scan(&latest); err != nil || latest != 20261026000000 {
			t.Errorf("%s: latest %d, %v; want the baseline", table, latest, err)
		}
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("every chain on the migrated database: %v", err)
	}
}

// TestMigrationEngineeringAndRuntimeSchemas_Down_PutsTheStatesBack: the
// tables go back to public, each Tag with its value and Quality; a Tag
// without a state gets its type's zero value with Quality bad. The public
// migration goes back only from the baseline of the roles' chains, so they
// are rolled back to it first (ADR 0011).
func TestMigrationEngineeringAndRuntimeSchemas_Down_PutsTheStatesBack(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const main = "b0000000-0000-4000-8000-000000000001"
	written, stateless := uuid.New(), uuid.New()
	if _, err := db.ExecContext(ctx, `INSERT INTO engineering.tags (id, project_id, name, type)
	                                  VALUES ($1, $3, 'speed', 'integer'), ($2, $3, 'flag', 'boolean')`, written, stateless, main); err != nil {
		t.Fatalf("insert tag definitions: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.tags (id, project_id, name, type, version)
	                                  VALUES ($1, $2, 'speed', 'integer', 1)`, written, main); err != nil {
		t.Fatalf("insert the copy of the tag definition: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.tag_states (tag_id, value, quality, seq)
	                                  VALUES ($1, '5', 'good', 9)`, written); err != nil {
		t.Fatalf("insert tag state: %v", err)
	}

	rollRolesBackToBaseline(t, ctx, db, dir)
	if err := goose.DownToContext(ctx, db, dir, 20261025000000); err != nil {
		t.Fatalf("migrate down: %v", err)
	}

	got := map[uuid.UUID]string{}
	rows, err := db.QueryContext(ctx, `SELECT id, value || '/' || quality FROM public.tags`)
	if err != nil {
		t.Fatalf("public.tags: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tagID uuid.UUID
		var s string
		if err := rows.Scan(&tagID, &s); err != nil {
			t.Fatal(err)
		}
		got[tagID] = s
	}
	if got[written] != "5/good" || got[stateless] != "false/bad" {
		t.Errorf("states back in the tags: %v", got)
	}
	var schemas int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname IN ('engineering', 'runtime')`).Scan(&schemas); err != nil || schemas != 0 {
		t.Errorf("schemas of the roles left: %d, %v", schemas, err)
	}
}

// TestMigrationTagStatesAndDevices_SplitsTheStateAndAddsDevices: the Tag
// state moves to runtime.tag_states, the copy of the definition keeps only
// what a write checks, and a state never outlives its definition; the
// Devices, the Environments and the parameters of a pair get a table of the
// Draft and a copy in the Runtime schema (task 34, ADR 0011 amendment
// 2026-11-01).
func TestMigrationTagStatesAndDevices_SplitsTheStateAndAddsDevices(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	const main = "b0000000-0000-4000-8000-000000000001"
	if err := goose.UpToContext(ctx, db, dir, rolesBaseline); err != nil {
		t.Fatalf("migrate public: %v", err)
	}
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	tagID, deviceID, environmentID := uuid.New(), uuid.New(), uuid.New()
	mustExec(`INSERT INTO runtime.tags (id, project_id, name, type, version, value, quality, seq)
	          VALUES ($1, $2, 'speed', 'integer', 3, '42', 'uncertain', 7)`, tagID, main)
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("migrate the chains of the roles: %v", err)
	}

	var value, quality string
	var seq int64
	if err := db.QueryRowContext(ctx, `SELECT value, quality, seq FROM runtime.tag_states WHERE tag_id = $1`, tagID).
		Scan(&value, &quality, &seq); err != nil || value != "42" || quality != "uncertain" || seq != 7 {
		t.Errorf("the state moved: %q/%q at %d, %v", value, quality, seq, err)
	}
	var stateColumns int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema = 'runtime'
	                                   AND table_name = 'tags' AND column_name IN ('value', 'quality', 'seq')`).Scan(&stateColumns); err != nil || stateColumns != 0 {
		t.Errorf("state columns left in the copy of the definition: %d, %v", stateColumns, err)
	}
	mustExec(`DELETE FROM runtime.tags WHERE id = $1`, tagID)
	var states int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runtime.tag_states`).Scan(&states); err != nil || states != 0 {
		t.Errorf("states left after their definitions went: %d, %v", states, err)
	}

	// The Draft: a Device and an Environment of the Project, one pair of
	// them, a name unique in the Project, a Tag's supplier among the Devices.
	mustExec(`INSERT INTO engineering.devices (id, project_id, name, protocol, config)
	          VALUES ($1, $2, 'sim', 'simulation', '{"pattern":"sine"}')`, deviceID, main)
	mustExec(`INSERT INTO engineering.environments (id, project_id, name) VALUES ($1, $2, 'shop')`, environmentID, main)
	mustExec(`INSERT INTO engineering.environment_devices (environment_id, device_id, params)
	          VALUES ($1, $2, '{"address":"localhost:9000"}')`, environmentID, deviceID)
	mustExec(`INSERT INTO engineering.tags (id, project_id, name, type, device_id)
	          VALUES ($1, $2, 'speed', 'integer', $3)`, tagID, main, deviceID)
	if _, err := db.ExecContext(ctx, `INSERT INTO engineering.environments (id, project_id, name) VALUES ($1, $2, 'shop')`,
		uuid.New(), main); err == nil {
		t.Error("an environment name taken twice in the project")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO engineering.environment_devices (environment_id, device_id) VALUES ($1, $2)`,
		environmentID, deviceID); err == nil {
		t.Error("a pair (environment, device) taken twice")
	}
	var pairIndex int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = 'engineering'
	                                   AND tablename = 'environment_devices' AND indexname = 'idx_environment_devices_device_id'`).Scan(&pairIndex); err != nil || pairIndex != 1 {
		t.Errorf("the index of the pairs by device: %d, %v", pairIndex, err)
	}

	// The copies in the Runtime schema: the Devices and the pairs, and one
	// token per Device, looked up by its hash. Nothing is copied before the
	// bridge of task 35 fills them.
	for _, table := range []string{"runtime.devices", "runtime.environments", "runtime.environment_devices", "runtime.device_tokens"} {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d rows before the copy arrived, %v", table, n, err)
		}
	}
	mustExec(`INSERT INTO runtime.devices (id, project_id, name, protocol) VALUES ($1, $2, 'sim', 'simulation')`, deviceID, main)
	mustExec(`INSERT INTO runtime.device_tokens (device_id, project_id, token_hash, fingerprint, issued_at)
	          VALUES ($1, $2, 'hash', 'fingerprint', now())`, deviceID, main)
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.device_tokens (device_id, project_id, token_hash, fingerprint, issued_at)
	                                   VALUES ($1, $2, 'other', 'fingerprint', now())`, deviceID, main); err == nil {
		t.Error("a second token of one device")
	}

	// A Gateway names the Environment it serves, so the copy resolves that
	// name to the pair with the Device inside the Project (and nowhere else).
	mustExec(`INSERT INTO runtime.environments (id, project_id, name) VALUES ($1, $2, 'shop')`, environmentID, main)
	mustExec(`INSERT INTO runtime.environment_devices (environment_id, device_id, params)
	          VALUES ($1, $2, '{"address":"localhost:9000"}')`, environmentID, deviceID)
	var address string
	if err := db.QueryRowContext(ctx, `SELECT e.params->>'address' FROM runtime.environments n
	                                    JOIN runtime.environment_devices e ON e.environment_id = n.id
	                                    WHERE n.project_id = $1 AND n.name = 'shop' AND e.device_id = $2`,
		main, deviceID).Scan(&address); err != nil || address != "localhost:9000" {
		t.Errorf("the parameters of the pair named by its Environment: %q, %v", address, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.environments (id, project_id, name) VALUES ($1, $2, 'shop')`,
		uuid.New(), main); err == nil {
		t.Error("an Environment name taken twice in the Project's copy")
	}

	// A token sleeps while its Device is not in the running Deployment: Deploy
	// removes the copy of the Device, the token stays behind (ADR 0004,
	// amendment 2026-11-01).
	mustExec(`DELETE FROM runtime.devices WHERE id = $1`, deviceID)
	var tokens int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runtime.device_tokens WHERE device_id = $1`, deviceID).Scan(&tokens); err != nil || tokens != 1 {
		t.Errorf("the token of a Device out of the Deployment: %d, %v", tokens, err)
	}
	// A pair is derived data too: it goes with either side, while the other
	// side and the token stay (ADR 0011). The pair of the Device is gone...
	var pairs int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runtime.environment_devices`).Scan(&pairs); err != nil || pairs != 0 {
		t.Errorf("pairs left after their Device went: %d, %v", pairs, err)
	}
	var environments int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runtime.environments WHERE id = $1`, environmentID).Scan(&environments); err != nil || environments != 1 {
		t.Errorf("the Environment went with the Device's pair: %d, %v", environments, err)
	}
	// ...and so is the pair of the Environment.
	otherDeviceID := uuid.New()
	mustExec(`INSERT INTO runtime.devices (id, project_id, name, protocol) VALUES ($1, $2, 'sim-2', 'simulation')`, otherDeviceID, main)
	mustExec(`INSERT INTO runtime.environment_devices (environment_id, device_id) VALUES ($1, $2)`, environmentID, otherDeviceID)
	mustExec(`DELETE FROM runtime.environments WHERE id = $1`, environmentID)
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runtime.environment_devices`).Scan(&pairs); err != nil || pairs != 0 {
		t.Errorf("pairs left after their Environment went: %d, %v", pairs, err)
	}
	var devices int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runtime.devices WHERE id = $1`, otherDeviceID).Scan(&devices); err != nil || devices != 1 {
		t.Errorf("the Device went with the Environment's pair: %d, %v", devices, err)
	}

	// Down: the state goes back into the copy of the definition.
	mustExec(`INSERT INTO runtime.tags (id, project_id, name, type, version) VALUES ($1, $2, 'speed', 'integer', 3)`, tagID, main)
	mustExec(`INSERT INTO runtime.tag_states (tag_id, value, quality, seq) VALUES ($1, '5', 'good', 9)`, tagID)
	rollRolesBackToBaseline(t, ctx, db, dir)
	if err := db.QueryRowContext(ctx, `SELECT value || '/' || quality FROM runtime.tags WHERE id = $1`, tagID).Scan(&value); err != nil || value != "5/good" {
		t.Errorf("the state back in the copy of the definition: %q, %v", value, err)
	}
	var seqBack int64
	if err := db.QueryRowContext(ctx, `SELECT seq FROM runtime.tags WHERE id = $1`, tagID).Scan(&seqBack); err != nil || seqBack != 9 {
		t.Errorf("the seq back in the copy of the definition: %d, %v", seqBack, err)
	}
}

// TestMigrationLibraries_PutsExistingWidgetTypesIntoMain: the WidgetTypes
// stored so far become the Draft of the Library "Main", names taken twice
// suffixed, released once and pinned in every live Project; Widgets lose the
// foreign key to the Draft; the outbox rows of WidgetTypes name the Library
// and the waiting reconciliation jobs keep only their Scene, those of every
// Scene of a WidgetType become jobs of the Projects holding a Widget of it
// (ADR 0007).
func TestMigrationLibraries_PutsExistingWidgetTypesIntoMain(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	if err := goose.UpToContext(ctx, db, dir, 20261026000000); err != nil {
		t.Fatalf("migrate public: %v", err)
	}
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	const main = "b0000000-0000-4000-8000-000000000001"
	deleted, first, second, taken, sceneID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mustExec(`INSERT INTO projects (id, organization_id, name, deleted_at)
	          SELECT $1, id, 'gone', now() FROM organizations`, deleted)
	mustExec(`INSERT INTO engineering.widget_types (id, name, html_template, script, script_language)
	          VALUES ($1, 'gauge', '', '', 'javascript'), ($2, 'gauge', '', '', 'javascript'), ($3, 'gauge (2)', '', '', 'javascript')`,
		first, second, taken)
	mustExec(`INSERT INTO engineering.scenes (id, project_id, name) VALUES ($1, $2, 's')`, sceneID, main)
	mustExec(`INSERT INTO engineering.widgets (id, project_id, name, type_id, scene_id) VALUES ($1, $2, 'w', $3, $4)`, uuid.New(), main, second, sceneID)
	mustExec(`INSERT INTO outbox (type, payload) VALUES ('widget_type_updated', $1)`,
		`{"timestamp":"2026-10-27T12:00:00Z","id":"`+first.String()+`"}`)
	mustExec(`INSERT INTO policy_jobs (policy, job_key) VALUES
	          ('reconcile_widgets_with_widget_type', 'widget_type:` + first.String() + `'),
	          ('reconcile_widgets_with_widget_type', 'widget_type:` + second.String() + `'),
	          ('reconcile_widgets_with_widget_type', 'widget_type:` + taken.String() + `'),
	          ('reconcile_widgets_with_widget_type', 'widget_type:` + first.String() + `/project:` + main + `/scene:` + sceneID.String() + `'),
	          ('reconcile_widgets_with_widget_type', 'widget_type:` + second.String() + `/project:` + main + `/scene:` + sceneID.String() + `')`)

	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(filepath.Join(dir, "..", "engineering")),
		goose.WithTableName("engineering.goose_db_version"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 20261027000000); err != nil {
		t.Fatalf("migrate engineering: %v", err)
	}

	names := map[uuid.UUID]string{}
	rows, err := db.QueryContext(ctx, `SELECT w.id, w.name FROM engineering.widget_types w
	                                   JOIN engineering.libraries l ON l.id = w.library_id WHERE l.name = 'Main'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		names[id] = name
	}
	rows.Close()
	if names[first] != "gauge" || names[second] != "gauge (3)" || names[taken] != "gauge (2)" {
		t.Errorf("the Draft of Main: %v", names)
	}
	var released, pins, pinsOfDeleted int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.released_widget_types WHERE release_number = 1`).Scan(&released); err != nil || released != 3 {
		t.Errorf("release 1 holds %d types (%v), want 3", released, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE project_id = $1), count(*) FILTER (WHERE project_id = $2)
	                                   FROM engineering.library_pins`, main, deleted).Scan(&pins, &pinsOfDeleted); err != nil || pins != 1 || pinsOfDeleted != 0 {
		t.Errorf("pins: %d in Main, %d in a deleted project (%v); want 1 and 0", pins, pinsOfDeleted, err)
	}
	var libraryID string
	if err := db.QueryRowContext(ctx, `SELECT payload->>'library_id' FROM outbox`).Scan(&libraryID); err != nil || libraryID != "d0000000-0000-4000-8000-000000000001" {
		t.Errorf("the outbox row names library %q (%v)", libraryID, err)
	}
	var jobs []string
	jobRows, err := db.QueryContext(ctx, `SELECT policy || ' ' || job_key FROM policy_jobs ORDER BY job_key`)
	if err != nil {
		t.Fatal(err)
	}
	for jobRows.Next() {
		var j string
		if err := jobRows.Scan(&j); err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, j)
	}
	jobRows.Close()
	if want := []string{
		"reconcile_widgets_with_pins project:" + main,
		"reconcile_widgets_with_pins project:" + main + "/scene:" + sceneID.String(),
	}; !slices.Equal(jobs, want) {
		t.Errorf("policy jobs %v, want %v", jobs, want)
	}
	// A Widget's type need no longer be in the Draft.
	mustExec(`DELETE FROM engineering.widget_types WHERE id = $1`, second)

	if _, err := provider.DownTo(ctx, 20261026000000); err != nil {
		t.Fatalf("migrate engineering down: %v", err)
	}
	var widgets int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.widgets`).Scan(&widgets); err != nil || widgets != 0 {
		t.Errorf("down: a widget of a type gone from the Draft must go, %d left (%v)", widgets, err)
	}
}

// TestMigrationJournal_KeepsThePositionAboveEveryState: the Journal's
// sequence continues above every seq already handed out to a Tag state, so a
// client holding a state of the old numbering does not drop the next change of
// its Tag as the older one; a state gets its last write moment (NULL for the
// initial one), and the subscribers' cursors start empty (ADR 0019).
func TestMigrationJournal_KeepsThePositionAboveEveryState(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	const main = "b0000000-0000-4000-8000-000000000001"
	if err := goose.UpToContext(ctx, db, dir, rolesBaseline); err != nil {
		t.Fatalf("migrate public: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(filepath.Join(dir, "..", "runtime")),
		goose.WithTableName("runtime.goose_db_version"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 20261105000001); err != nil {
		t.Fatalf("migrate runtime to the previous version: %v", err)
	}
	tagID := uuid.New()
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.tags (id, project_id, name, type, version)
	                                  VALUES ($1, $2, 'speed', 'integer', 1)`, tagID, main); err != nil {
		t.Fatalf("insert the copy of a tag definition: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.tag_states (tag_id, value, quality, seq)
	                                  VALUES ($1, '42', 'good', 42)`, tagID); err != nil {
		t.Fatalf("insert a tag state: %v", err)
	}

	if _, err := provider.UpTo(ctx, 20261106000000); err != nil {
		t.Fatalf("migrate the journal: %v", err)
	}

	var next int64
	if err := db.QueryRowContext(ctx, `SELECT nextval('runtime.journal_seq')`).Scan(&next); err != nil {
		t.Fatalf("next journal position: %v", err)
	}
	if next <= 42 {
		t.Errorf("the first journal position is %d, not above the state's 42", next)
	}
	var writtenAt sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT last_write_at FROM runtime.tag_states WHERE tag_id = $1`, tagID).Scan(&writtenAt); err != nil {
		t.Fatalf("read last_write_at: %v", err)
	}
	if writtenAt.Valid {
		t.Errorf("a state written before the journal has a write moment: %s", writtenAt.Time)
	}
	var sequences int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
	                                   WHERE n.nspname = 'runtime' AND c.relkind = 'S' AND c.relname IN ('tag_state_seq', 'journal_seq')`).Scan(&sequences); err != nil || sequences != 1 {
		t.Errorf("the sequences of the runtime schema: %d, %v; want journal_seq alone", sequences, err)
	}
	var cursors int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runtime.journal_cursors`).Scan(&cursors); err != nil || cursors != 0 {
		t.Errorf("the cursors of the subscribers: %d, %v; want none", cursors, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.journal (project_id, ts, type, payload, schema_version)
	                                  VALUES ($1, now(), 'revision', '{"number":1,"cause":"deploy"}', '0.2')`, main); err != nil {
		t.Errorf("insert a revision entry: %v", err)
	}

	// Down: the state's seq is a number of its own sequence again, set above
	// every stored one; the Journal and the write moment are gone.
	if _, err := provider.DownTo(ctx, 20261105000001); err != nil {
		t.Fatalf("migrate the journal down: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT nextval('runtime.tag_state_seq')`).Scan(&next); err != nil {
		t.Fatalf("next state position: %v", err)
	}
	if next <= 42 {
		t.Errorf("the first state position is %d, not above the stored 42", next)
	}
	var journalTables, writeMoment int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = 'runtime'
	                                   AND tablename IN ('journal', 'journal_cursors')`).Scan(&journalTables); err != nil || journalTables != 0 {
		t.Errorf("journal tables left after the down: %d, %v", journalTables, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema = 'runtime'
	                                   AND table_name = 'tag_states' AND column_name = 'last_write_at'`).Scan(&writeMoment); err != nil || writeMoment != 0 {
		t.Errorf("last_write_at left after the down: %d, %v", writeMoment, err)
	}
}

// TestMigrationDeployments_RenamesTheRevisionStore: the Runtime node's store of
// a Project's running configuration was called a Revision before task 72_9. A
// row written then is read as a Deployment after the migration, its Journal
// entry is a deployment entry, and the Down puts both names back.
func TestMigrationDeployments_RenamesTheRevisionStore(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	const main = "b0000000-0000-4000-8000-000000000001"
	if err := goose.UpToContext(ctx, db, dir, rolesBaseline); err != nil {
		t.Fatalf("migrate public: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(filepath.Join(dir, "..", "runtime")),
		goose.WithTableName("runtime.goose_db_version"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 20261106000000); err != nil {
		t.Fatalf("migrate runtime to the previous version: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.revisions (project_id, n, deployed_at, cause, config)
	                                  VALUES ($1, 1, now(), 'deploy', '{}')`, main); err != nil {
		t.Fatalf("insert a revision: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.journal (project_id, ts, type, payload, schema_version)
	                                  VALUES ($1, now(), 'revision', '{"number":1,"cause":"deploy"}', '0.2')`, main); err != nil {
		t.Fatalf("insert a revision journal entry: %v", err)
	}

	if _, err := provider.UpTo(ctx, 20261107000000); err != nil {
		t.Fatalf("migrate the deployment store: %v", err)
	}

	var number int
	if err := db.QueryRowContext(ctx, `SELECT number FROM runtime.deployments WHERE project_id = $1`, main).Scan(&number); err != nil {
		t.Fatalf("read the stored deployment: %v", err)
	}
	if number != 1 {
		t.Errorf("the stored deployment is numbered %d, want 1", number)
	}
	var entryType string
	if err := db.QueryRowContext(ctx, `SELECT type FROM runtime.journal WHERE project_id = $1`, main).Scan(&entryType); err != nil {
		t.Fatalf("read the journal entry: %v", err)
	}
	if entryType != "deployment" {
		t.Errorf("the journal entry is of type %q, want deployment", entryType)
	}
	var oldTables int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = 'runtime' AND tablename = 'revisions'`).Scan(&oldTables); err != nil || oldTables != 0 {
		t.Errorf("the revisions table is left: %d, %v", oldTables, err)
	}

	// Down: the store and the Journal entry are named as they were.
	if _, err := provider.DownTo(ctx, 20261106000000); err != nil {
		t.Fatalf("migrate the deployment store down: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT n FROM runtime.revisions WHERE project_id = $1`, main).Scan(&number); err != nil {
		t.Fatalf("read the stored revision after the down: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT type FROM runtime.journal WHERE project_id = $1`, main).Scan(&entryType); err != nil {
		t.Fatalf("read the journal entry after the down: %v", err)
	}
	if entryType != "revision" {
		t.Errorf("the journal entry after the down is of type %q, want revision", entryType)
	}
}

// TestMigrationLabels_SeedsTwoOrdinaryLabels: a migrated database carries the
// two starter labels of the one Organization, with no color (task 85_2;
// ADR 0025). They are ordinary rows, not built-in behavior.
func TestMigrationLabels_SeedsTwoOrdinaryLabels(t *testing.T) {
	ctx := context.Background()
	db, _ := migrationDB(t)
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT l.name, l.color FROM engineering.labels l
		 JOIN organizations o ON o.id = l.organization_id ORDER BY name`)
	if err != nil {
		t.Fatalf("read the seed labels: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		var color sql.NullString
		if err := rows.Scan(&name, &color); err != nil {
			t.Fatalf("scan the seed label: %v", err)
		}
		if color.Valid {
			t.Errorf("the seed label %q has color %q, want none", name, color.String)
		}
		names = append(names, name)
	}
	if strings.Join(names, ",") != "ready_for_deploy,ready_for_import" {
		t.Errorf("the seed labels: %v, want ready_for_deploy and ready_for_import", names)
	}
}

// TestMigrationDeploymentCommitNumber_ReadsTheSavedStatesBack: a Deployment
// names the saved state it deployed, not the Release that published it (task
// 85_4). The backfill reads the commit-backed Release of each Deployment, the
// Journal entries and the Checkpoints are rewritten to the record's 0.6 and the
// Release column goes; the Down puts the Release numbers back.
func TestMigrationDeploymentCommitNumber_ReadsTheSavedStatesBack(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	const main = "b0000000-0000-4000-8000-000000000001"
	if err := goose.UpToContext(ctx, db, dir, rolesBaseline); err != nil {
		t.Fatalf("migrate public: %v", err)
	}
	engineering, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(filepath.Join(dir, "..", "engineering")),
		goose.WithTableName("engineering.goose_db_version"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engineering.UpTo(ctx, 20261109000001); err != nil {
		t.Fatalf("migrate engineering to the project releases: %v", err)
	}
	deployments, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(filepath.Join(dir, "..", "runtime")),
		goose.WithTableName("runtime.goose_db_version"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deployments.UpTo(ctx, 20261111000000); err != nil {
		t.Fatalf("migrate runtime to the checkpoints: %v", err)
	}
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	mustExec(`INSERT INTO projects (id, organization_id, name)
	          SELECT $1, id, 'Main' FROM organizations
	          ON CONFLICT (id) DO NOTHING`, main)
	// Release 1 published Commit 7; Release 2 came from a file and names none.
	mustExec(`INSERT INTO engineering.project_releases
	          (id, project_id, number, commit_number, commit_title, note, author, created_at, origin, content)
	          VALUES ($1, $2, 1, 7, 'Ready', NULL, 'Ada', now(), 'commit', '{}'),
	                 ($3, $2, 2, NULL, NULL, NULL, 'file', now(), 'file', '{}')`, uuid.New(), main, uuid.New())
	// Deployment 1 deployed Release 1; Deployment 2 deployed the file Release 2;
	// Deployment 3 is a Rollback that copied Deployment 2.
	mustExec(`INSERT INTO runtime.deployments (project_id, number, deployed_at, cause, rolled_back_from, release_number, config)
	          VALUES ($1, 1, now(), 'deploy', NULL, 1, '{}'),
	                 ($1, 2, now(), 'deploy', NULL, 2, '{}'),
	                 ($1, 3, now(), 'rollback', 2, 2, '{}')`, main)
	mustExec(`INSERT INTO runtime.journal (project_id, ts, type, payload, schema_version)
	          VALUES ($1, now(), 'tag_state', '{"tag_id":"11111111-1111-1111-1111-111111111111","value":"1","quality":"good"}', '0.5'),
	                 ($1, now(), 'deployment', '{"number":1,"cause":"deploy","release_number":1}', '0.5'),
	                 ($1, now(), 'deployment', '{"number":2,"cause":"deploy","release_number":2}', '0.5'),
	                 ($1, now(), 'deployment', '{"number":3,"cause":"rollback","rolled_back_from":2,"release_number":2}', '0.5')`, main)
	mustExec(`INSERT INTO runtime.checkpoints (project_id, seq, taken_at, schema_version, payload)
	          VALUES ($1, 3, now(), '0.5', '{"deployment":{"number":1,"cause":"deploy","release_number":1},"tag_states":[]}'),
	                 ($1, 4, now(), '0.5', '{"deployment":{"number":2,"cause":"deploy","release_number":2},"tag_states":[]}')`, main)

	if _, err := deployments.UpTo(ctx, 20261114000002); err != nil {
		t.Fatalf("migrate the saved state of a Deployment: %v", err)
	}

	for number, want := range map[int]sql.NullInt64{1: {Int64: 7, Valid: true}, 2: {}, 3: {}} {
		var got sql.NullInt64
		if err := db.QueryRowContext(ctx,
			`SELECT commit_number FROM runtime.deployments WHERE project_id = $1 AND number = $2`, main, number,
		).Scan(&got); err != nil {
			t.Fatalf("read the deployment %d: %v", number, err)
		}
		if got.Valid != want.Valid || (got.Valid && got.Int64 != want.Int64) {
			t.Errorf("deployment %d names saved state %v, want %v", number, got, want)
		}
	}
	var columns int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM information_schema.columns
		 WHERE table_schema = 'runtime' AND table_name = 'deployments' AND column_name = 'release_number'`,
	).Scan(&columns); err != nil || columns != 0 {
		t.Errorf("the Release column is left: %d, %v", columns, err)
	}
	var journalCommit, journalSchema string
	var journalRelease sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT payload ->> 'commit_number', schema_version, payload ->> 'release_number'
		 FROM runtime.journal WHERE project_id = $1 AND payload ->> 'number' = '1'`, main,
	).Scan(&journalCommit, &journalSchema, &journalRelease); err != nil {
		t.Fatalf("read the journal entry: %v", err)
	}
	if journalCommit != "7" || journalSchema != "0.6" || journalRelease.Valid {
		t.Errorf("the journal entry: saved state %q, schema %q, release %v; want 7, 0.6 and none",
			journalCommit, journalSchema, journalRelease)
	}
	var fileRelease sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT payload ->> 'release_number' FROM runtime.journal
		 WHERE project_id = $1 AND payload ->> 'number' = '2'`, main,
	).Scan(&fileRelease); err != nil || fileRelease.Valid {
		t.Errorf("the file deployment's entry keeps a Release: %v, %v", fileRelease, err)
	}
	var tagSchema string
	if err := db.QueryRowContext(ctx,
		`SELECT schema_version FROM runtime.journal WHERE project_id = $1 AND type = 'tag_state'`, main,
	).Scan(&tagSchema); err != nil || tagSchema != "0.5" {
		t.Errorf("a tag_state entry changed its schema: %q, %v", tagSchema, err)
	}
	var checkpointCommit, checkpointSchema string
	var checkpointRelease sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT payload -> 'deployment' ->> 'commit_number', schema_version, payload -> 'deployment' ->> 'release_number'
		 FROM runtime.checkpoints WHERE project_id = $1 AND seq = 3`, main,
	).Scan(&checkpointCommit, &checkpointSchema, &checkpointRelease); err != nil {
		t.Fatalf("read the checkpoint: %v", err)
	}
	if checkpointCommit != "7" || checkpointSchema != "0.6" || checkpointRelease.Valid {
		t.Errorf("the checkpoint: saved state %q, schema %q, release %v; want 7, 0.6 and none",
			checkpointCommit, checkpointSchema, checkpointRelease)
	}
	var fileCheckpointCommit sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT payload -> 'deployment' ->> 'commit_number'
		 FROM runtime.checkpoints WHERE project_id = $1 AND seq = 4`, main,
	).Scan(&fileCheckpointCommit); err != nil || fileCheckpointCommit.Valid {
		t.Errorf("the file checkpoint names saved state %v: %v", fileCheckpointCommit, err)
	}

	// Down: the Release numbers come back from the Releases, and the payloads
	// are the record's 0.5 again.
	if _, err := deployments.DownTo(ctx, 20261111000000); err != nil {
		t.Fatalf("migrate the saved state of a Deployment down: %v", err)
	}
	// A file Deployment named a Release made from a file, and a file Release
	// names no Commit: the down restores the commit-backed ones alone (the
	// file Deploy of task 85_4 leaves no Release to name).
	for number, want := range map[int]sql.NullInt64{1: {Int64: 1, Valid: true}, 2: {}, 3: {}} {
		var got sql.NullInt64
		if err := db.QueryRowContext(ctx,
			`SELECT release_number FROM runtime.deployments WHERE project_id = $1 AND number = $2`, main, number,
		).Scan(&got); err != nil {
			t.Fatalf("read deployment %d after the down: %v", number, err)
		}
		if got.Valid != want.Valid || (got.Valid && got.Int64 != want.Int64) {
			t.Errorf("deployment %d names release %v after the down, want %v", number, got, want)
		}
	}
	if err := db.QueryRowContext(ctx,
		`SELECT payload ->> 'release_number', schema_version
		 FROM runtime.journal WHERE project_id = $1 AND payload ->> 'number' = '1'`, main,
	).Scan(&journalRelease, &journalSchema); err != nil {
		t.Fatalf("read the journal entry after the down: %v", err)
	}
	if !journalRelease.Valid || journalRelease.String != "1" || journalSchema != "0.5" {
		t.Errorf("the journal entry after the down: release %v, schema %q, want 1 and 0.5", journalRelease, journalSchema)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT payload -> 'deployment' ->> 'release_number', schema_version
		 FROM runtime.checkpoints WHERE project_id = $1 AND seq = 3`, main,
	).Scan(&checkpointRelease, &checkpointSchema); err != nil {
		t.Fatalf("read the checkpoint after the down: %v", err)
	}
	if !checkpointRelease.Valid || checkpointRelease.String != "1" || checkpointSchema != "0.5" {
		t.Errorf("the checkpoint after the down: release %v, schema %q, want 1 and 0.5", checkpointRelease, checkpointSchema)
	}
}

// TestMigrationLibraryImports_TurnsPinsIntoImports: a Project imports the saved
// state a commit-backed Release published rather than pinning that Release
// (task 85_6). The import carries the Commit's identity, a legacy Release goes
// with its copies and the Widgets of them, the saved states, the Releases and
// every Deployment document are rewritten to library_imports, and the Draft
// whose content changed reports unsaved changes.
func TestMigrationLibraryImports_TurnsPinsIntoImports(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	const main = "b0000000-0000-4000-8000-000000000001"
	if err := goose.UpToContext(ctx, db, dir, rolesBaseline); err != nil {
		t.Fatalf("migrate public: %v", err)
	}
	engineering, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(filepath.Join(dir, "..", "engineering")),
		goose.WithTableName("engineering.goose_db_version"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engineering.UpTo(ctx, 20261114000001); err != nil {
		t.Fatalf("migrate engineering to the commits: %v", err)
	}
	deployments, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(filepath.Join(dir, "..", "runtime")),
		goose.WithTableName("runtime.goose_db_version"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deployments.UpTo(ctx, 20261114000002); err != nil {
		t.Fatalf("migrate runtime: %v", err)
	}
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE projects SET name = 'Imports' WHERE id = $1`, main); err != nil {
		t.Fatalf("name the project: %v", err)
	}
	// Two Libraries: A was saved and released from its Commit; B's release 1
	// is a legacy one, without a Commit.
	committedLibrary, legacyLibrary := uuid.New(), uuid.New()
	keptType, droppedType := uuid.New(), uuid.New()
	keptWidget, droppedWidget, sceneID, commitID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mustExec(`INSERT INTO engineering.libraries (id, organization_id, name, version)
	          SELECT $1, id, 'Kept', 1 FROM organizations`, committedLibrary)
	mustExec(`INSERT INTO engineering.libraries (id, organization_id, name, version)
	          SELECT $1, id, 'Dropped', 1 FROM organizations`, legacyLibrary)
	mustExec(`UPDATE engineering.drafts SET version = 5, clean_version = 5 WHERE project_id = $1`, main)
	mustExec(`INSERT INTO engineering.drafts (id, library_id, version) VALUES ($1, $2, 3)`, uuid.New(), committedLibrary)
	mustExec(`INSERT INTO engineering.drafts (id, library_id, version) VALUES ($1, $2, 2)`, uuid.New(), legacyLibrary)
	mustExec(`INSERT INTO engineering.commits (id, draft_id, number, title, author, created_at, draft_version, content)
	          SELECT $1, d.id, 1, 'Save', 'Ada', now(), 3,
	                 jsonb_build_object('format', 1, 'kind', 'library', 'widget_types',
	                     jsonb_build_array(jsonb_build_object(
	                         'id', $2::uuid, 'library_id', $3::uuid, 'name', 'gauge',
	                         'html_template', '', 'script', '', 'script_language', 'javascript',
	                         'default_width', 10, 'default_height', 10, 'input_ports', '[]'::jsonb)))
	          FROM engineering.drafts d WHERE d.library_id = $3`, commitID, keptType, committedLibrary)
	mustExec(`INSERT INTO engineering.library_releases (library_id, number, note, commit_number, author, origin)
	          VALUES ($1, 1, '', 1, 'Ada', 'commit'), ($2, 1, '', NULL, '', 'legacy')`, committedLibrary, legacyLibrary)
	mustExec(`INSERT INTO engineering.released_widget_types
	              (library_id, release_number, id, name, html_template, script, script_language, default_width, default_height, input_ports)
	          VALUES ($1, 1, $2, 'gauge', '', '', 'javascript', 10, 10, '[]'),
	                 ($3, 1, $4, 'gauge', '', '', 'javascript', 10, 10, '[]')`,
		committedLibrary, keptType, legacyLibrary, droppedType)
	mustExec(`INSERT INTO engineering.library_pins (project_id, library_id, release_number)
	          VALUES ($1, $2, 1), ($1, $3, 1)`, main, committedLibrary, legacyLibrary)
	mustExec(`INSERT INTO engineering.scenes (id, project_id, name, width, height, background_html)
	          VALUES ($1, $2, 'Main', 100, 100, '')`, sceneID, main)
	mustExec(`INSERT INTO engineering.widgets (id, project_id, name, type_id, scene_id, labels, port_bindings)
	          VALUES ($1, $3, 'kept', $4, $5, '{}', '[]'), ($2, $3, 'dropped', $6, $5, '{}', '[]')`,
		keptWidget, droppedWidget, main, keptType, sceneID, droppedType)
	mustExec(`INSERT INTO engineering.draft_changes (draft_id, seq, description, before, after)
	          SELECT id, 1, 'a change', '{"format":1,"entities":[]}', '{"format":1,"entities":[]}'
	          FROM engineering.drafts WHERE project_id = $1`, main)
	projectFile := `jsonb_build_object(
	        'schema_version', '0.2', 'project_id', $1::uuid,
	        'tags', '[]'::jsonb, 'devices', '[]'::jsonb, 'environments', '[]'::jsonb,
	        'environment_devices', '[]'::jsonb,
	        'library_pins', jsonb_build_array(
	            jsonb_build_object('library_id', $2::uuid, 'release', 1),
	            jsonb_build_object('library_id', $3::uuid, 'release', 1)),
	        'widget_types', jsonb_build_array(
	            jsonb_build_object('id', $4::uuid, 'library_id', $2::uuid, 'name', 'gauge'),
	            jsonb_build_object('id', $5::uuid, 'library_id', $3::uuid, 'name', 'gauge')),
	        'scenes', jsonb_build_array(jsonb_build_object(
	            'id', $6::uuid, 'name', 'Main', 'width', 100, 'height', 100,
	            'widgets', jsonb_build_array(
	                jsonb_build_object('id', $7::uuid, 'name', 'kept', 'type_id', $4::uuid),
	                jsonb_build_object('id', $8::uuid, 'name', 'dropped', 'type_id', $5::uuid)))))`
	args := []any{main, committedLibrary, legacyLibrary, keptType, droppedType, sceneID, keptWidget, droppedWidget}
	mustExec(`INSERT INTO engineering.commits (id, draft_id, number, title, author, created_at, draft_version, content)
	          SELECT $9, d.id, 1, 'Save', 'Ada', now(), 5,
	                 jsonb_build_object('format', 1, 'kind', 'project', 'project_file', `+projectFile+`)
	          FROM engineering.drafts d WHERE d.project_id = $1`,
		append(args, uuid.New())...)
	mustExec(`INSERT INTO engineering.project_releases
	              (id, project_id, number, commit_number, commit_title, note, author, created_at, origin, content)
	          VALUES ($9, $1, 1, 1, 'Save', NULL, 'Ada', now(), 'commit',
	                  jsonb_build_object('format', 1, 'kind', 'project', 'project_file', `+projectFile+`))`,
		append(args, uuid.New())...)
	mustExec(`INSERT INTO runtime.deployments (project_id, number, deployed_at, cause, config)
	          VALUES ($1, 1, now(), 'deploy', `+projectFile+`)`, args...)

	if _, err := engineering.UpTo(ctx, 20261114000002); err != nil {
		t.Fatalf("migrate the library imports: %v", err)
	}

	var commitNumber int
	var importedCommit uuid.UUID
	var imports int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*), coalesce(max(commit_number), 0), coalesce((array_agg(commit_id))[1], '00000000-0000-0000-0000-000000000000'::uuid) FROM engineering.library_imports WHERE project_id = $1`, main,
	).Scan(&imports, &commitNumber, &importedCommit); err != nil {
		t.Fatalf("read the imports: %v", err)
	}
	if imports != 1 || commitNumber != 1 || importedCommit != commitID {
		t.Errorf("the imports: %d rows, saved state %d, commit %s; want one of saved state 1 and %s",
			imports, commitNumber, importedCommit, commitID)
	}
	for _, query := range []string{
		`SELECT count(*) FROM engineering.library_releases WHERE library_id = '` + legacyLibrary.String() + `'`,
		`SELECT count(*) FROM engineering.released_widget_types WHERE id = '` + droppedType.String() + `'`,
		`SELECT count(*) FROM engineering.widgets WHERE id = '` + droppedWidget.String() + `'`,
	} {
		var left int
		if err := db.QueryRowContext(ctx, query).Scan(&left); err != nil || left != 0 {
			t.Errorf("%s: %d rows left, %v; want none", query, left, err)
		}
	}
	var version, cleanVersion, stackPosition int
	if err := db.QueryRowContext(ctx,
		`SELECT version, clean_version, stack_position FROM engineering.drafts WHERE project_id = $1`, main,
	).Scan(&version, &cleanVersion, &stackPosition); err != nil {
		t.Fatalf("read the draft: %v", err)
	}
	if version != 6 || cleanVersion != 0 || stackPosition != 0 {
		t.Errorf("the Draft: version %d, clean %d, stack %d; want 6, 0 and 0", version, cleanVersion, stackPosition)
	}
	var changes int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.draft_changes`).Scan(&changes); err != nil || changes != 0 {
		t.Errorf("the DraftChange stack: %d rows, %v; want none", changes, err)
	}

	// Every document names library_imports and holds no copy or Widget of the
	// dropped saved state.
	documents := map[string]string{
		"the saved state":        `SELECT (content -> 'project_file')::text FROM engineering.commits WHERE content ->> 'kind' = 'project'`,
		"the Release":            `SELECT (content -> 'project_file')::text FROM engineering.project_releases`,
		"the running Deployment": `SELECT config::text FROM runtime.deployments WHERE project_id = '` + main + `'`,
	}
	for what, query := range documents {
		var document string
		if err := db.QueryRowContext(ctx, query).Scan(&document); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		for _, want := range []string{`"library_imports"`, `"commit": 1`, `"name": "kept"`, `"schema_version": "0.3"`} {
			if !strings.Contains(document, want) {
				t.Errorf("%s does not hold %s: %s", what, want, document)
			}
		}
		for _, gone := range []string{"library_pins", `"schema_version": "0.2"`, "dropped", droppedType.String(), legacyLibrary.String()} {
			if strings.Contains(document, gone) {
				t.Errorf("%s still holds %s: %s", what, gone, document)
			}
		}
	}

	// Down: the shape comes back, and an import of a saved state this server
	// never released — the everyday case since task 85_6 — has no pin to
	// become, so it goes rather than failing the foreign key to the Releases.
	if _, err := db.ExecContext(ctx,
		`DELETE FROM engineering.library_releases WHERE library_id = $1`, committedLibrary); err != nil {
		t.Fatalf("drop the release of the imported saved state: %v", err)
	}
	if _, err := engineering.DownTo(ctx, 20261114000001); err != nil {
		t.Fatalf("down the library imports: %v", err)
	}
	var pins int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM engineering.library_pins WHERE project_id = $1`, main).Scan(&pins); err != nil || pins != 0 {
		t.Errorf("the pins after the down: %d, %v; want none", pins, err)
	}
}
