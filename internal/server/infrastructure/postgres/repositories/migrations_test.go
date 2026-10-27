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
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.tags d FULL JOIN runtime.tags s ON s.id = d.id
	                                   WHERE d.id IS NULL OR s.id IS NULL OR d.type <> s.type OR d.name <> s.name`).Scan(&unmatched); err != nil || unmatched != 0 {
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
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.library_pins`).Scan(&pins); err != nil || pins != 3 {
		t.Errorf("pins: %d, %v; want 3", pins, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.widgets w WHERE NOT EXISTS
	          (SELECT 1 FROM engineering.library_pins lp JOIN engineering.released_widget_types r
	           ON r.library_id = lp.library_id AND r.release_number = lp.release_number
	           WHERE lp.project_id = w.project_id AND r.id = w.type_id)`).Scan(&unpinnedWidgets); err != nil || unpinnedWidgets != 0 {
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
// without a state gets its type's zero value with Quality bad.
func TestMigrationEngineeringAndRuntimeSchemas_Down_PutsTheStatesBack(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	const main = "b0000000-0000-4000-8000-000000000001"
	written, stateless := uuid.New(), uuid.New()
	if _, err := db.ExecContext(ctx, `INSERT INTO engineering.tags (id, project_id, name, type, version)
	                                  VALUES ($1, $3, 'speed', 'integer', 1), ($2, $3, 'flag', 'boolean', 1)`, written, stateless, main); err != nil {
		t.Fatalf("insert tag definitions: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO runtime.tags (id, project_id, name, type, version, value, quality, seq)
	                                  VALUES ($1, $2, 'speed', 'integer', 1, '5', 'good', 9)`, written, main); err != nil {
		t.Fatalf("insert tag state: %v", err)
	}

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

// TestMigrationLibraries_PutsExistingWidgetTypesIntoMain: the WidgetTypes
// stored so far become the Draft of the Library "Main", names taken twice
// suffixed, released once and pinned in every live Project; Widgets lose the
// foreign key to the Draft; the outbox rows of WidgetTypes name the Library
// and the waiting reconciliation jobs keep only their Scene (ADR 0007).
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
	deleted, first, second, sceneID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mustExec(`INSERT INTO projects (id, organization_id, name, deleted_at)
	          SELECT $1, id, 'gone', now() FROM organizations`, deleted)
	mustExec(`INSERT INTO engineering.widget_types (id, name, html_template, script, script_language)
	          VALUES ($1, 'gauge', '', '', 'javascript'), ($2, 'gauge', '', '', 'javascript')`, first, second)
	mustExec(`INSERT INTO engineering.scenes (id, project_id, name) VALUES ($1, $2, 's')`, sceneID, main)
	mustExec(`INSERT INTO engineering.widgets (id, project_id, name, type_id, scene_id) VALUES ($1, $2, 'w', $3, $4)`, uuid.New(), main, second, sceneID)
	mustExec(`INSERT INTO outbox (type, payload) VALUES ('widget_type_updated', $1)`,
		`{"timestamp":"2026-10-27T12:00:00Z","id":"`+first.String()+`"}`)
	mustExec(`INSERT INTO policy_jobs (policy, job_key) VALUES
	          ('reconcile_widgets_with_widget_type', 'widget_type:`+first.String()+`'),
	          ('reconcile_widgets_with_widget_type', 'widget_type:`+second.String()+`/project:`+main+`/scene:`+sceneID.String()+`')`)

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
	if names[first] != "gauge" || names[second] != "gauge (2)" {
		t.Errorf("the Draft of Main: %v", names)
	}
	var released, pins, pinsOfDeleted int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM engineering.released_widget_types WHERE release_number = 1`).Scan(&released); err != nil || released != 2 {
		t.Errorf("release 1 holds %d types (%v), want 2", released, err)
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
	if want := "reconcile_widgets_with_pins project:" + main + "/scene:" + sceneID.String(); len(jobs) != 1 || jobs[0] != want {
		t.Errorf("policy jobs %v, want [%s]", jobs, want)
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
