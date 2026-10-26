package repositories_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
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
	return db, filepath.Join(filepath.Dir(currentFile), "..", "migrations")
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
// project Main, and the empty project Line 2.
func TestSeedData_AppliesOnTheMigratedSchema(t *testing.T) {
	ctx := context.Background()
	db, dir := migrationDB(t)
	if err := goose.UpContext(ctx, db, dir); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := goose.UpContext(ctx, db, filepath.Join(dir, "..", "test_data")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	const main = "b0000000-0000-4000-8000-000000000001"
	for _, table := range []string{"tags", "scenes", "widgets"} {
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
}
