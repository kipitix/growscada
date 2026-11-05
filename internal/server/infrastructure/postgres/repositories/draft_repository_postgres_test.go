package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// TestDraftUpdate_CAS: a write states the Version of the Draft; one that is
// not the stored one is refused and changes nothing (ADR 0016).
func TestDraftUpdate_CAS(t *testing.T) {
	cleanTags(t)
	cleanScenes(t)
	cleanEditLocks(t)
	ctx := editlock.System(context.Background())
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	created := makeTag(t, "speed", repo)

	first := draftVersionOf(t, mainProject)
	mustUpdate(t, mainProject, "create tag", func(session *draft.Session) error {
		session.CreateTag(ctx, created)
		return nil
	})
	if draftVersionOf(t, mainProject) != first.Next() {
		t.Fatalf("a write did not raise the Draft's version")
	}

	late := makeTag(t, "pressure", repo)
	_, err := testDrafts().Update(ctx, editlock.ProjectScope(mainProject), first, "create late tag",
		func(session *draft.Session) error {
			session.CreateTag(ctx, late)
			return nil
		})
	if !errors.Is(err, draft.ErrDraftConflict) {
		t.Fatalf("a stale write: %v, want ErrDraftConflict", err)
	}
	if _, err := repo.FindByID(ctx, late.ID()); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("the refused write stored its tag: %v", err)
	}
}

// TestDraftUpdate_WritesOneDraftChange: every successful write stores one row
// with the before and after states of what it touched, and moves the stack
// position (ADR 0017).
func TestDraftUpdate_WritesOneDraftChange(t *testing.T) {
	cleanTags(t)
	cleanScenes(t)
	cleanEditLocks(t)
	ctx := editlock.System(context.Background())
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	created := makeTag(t, "speed", repo)

	before := draftVersionOf(t, mainProject).Number()
	mustUpdate(t, mainProject, "Create tag speed", func(session *draft.Session) error {
		session.CreateTag(ctx, created)
		return nil
	})

	var (
		count       int
		seq         int64
		description string
		beforeJSON  []byte
		afterJSON   []byte
		draftStack  int64
	)
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*), max(seq), max(description), max(before::text)::jsonb, max(after::text)::jsonb
		 FROM engineering.draft_changes dc JOIN engineering.drafts d ON d.id = dc.draft_id
		 WHERE d.project_id = $1`, mainProject.UUID(),
	).Scan(&count, &seq, &description, &beforeJSON, &afterJSON); err != nil {
		t.Fatalf("read draft changes: %v", err)
	}
	if count != 1 || seq != 1 || description != "Create tag speed" {
		t.Errorf("draft changes: %d rows, last seq %d, %q; want one 'Create tag speed'", count, seq, description)
	}
	if err := testDB.QueryRowContext(ctx,
		`SELECT stack_position FROM engineering.drafts WHERE project_id = $1`, mainProject.UUID()).Scan(&draftStack); err != nil {
		t.Fatalf("read the stack position: %v", err)
	}
	if draftStack != 1 {
		t.Errorf("stack position %d, want 1", draftStack)
	}
	if before != 0 && len(beforeJSON) == 0 {
		t.Errorf("the change carries no before state: %s", beforeJSON)
	}
	if len(afterJSON) == 0 {
		t.Errorf("the change carries no after state")
	}
}

// TestDraftUpdate_Gate: a write of a Draft is refused to a client that does
// not hold its EditLock (ADR 0015, ADR 0016), and the holder's write raises
// last_seen.
func TestDraftUpdate_Gate(t *testing.T) {
	cleanTags(t)
	cleanEditLocks(t)
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	created := makeTag(t, "speed", repo)
	scope := editlock.ProjectScope(mainProject)
	version := draftVersionOf(t, mainProject)

	if _, err := testDrafts().Update(context.Background(), scope, version, "no client", func(*draft.Session) error {
		return nil
	}); !errors.Is(err, editlock.ErrNoClient) {
		t.Fatalf("a write with no client: %v, want ErrNoClient", err)
	}

	alice, err := editLockRepo().Create(editlock.System(context.Background()), createHeldLockOf(t, scope))
	if err != nil {
		t.Fatalf("hold the lock: %v", err)
	}
	bob := editlock.WithClient(context.Background(), editlock.NewKey())
	if _, err := testDrafts().Update(bob, scope, version, "another client", func(*draft.Session) error {
		return nil
	}); !errors.Is(err, editlock.ErrEditLockNotHeld) {
		t.Fatalf("a write of another client: %v, want ErrEditLockNotHeld", err)
	}

	holder := editlock.WithClient(context.Background(), alice.Holder().Key())
	if _, err := testDrafts().Update(holder, scope, version, "the holder", func(session *draft.Session) error {
		session.CreateTag(context.Background(), created)
		return nil
	}); err != nil {
		t.Fatalf("the holder's write: %v", err)
	}
	found, _, err := editLockRepo().Find(editlock.System(context.Background()), scope)
	if err != nil {
		t.Fatalf("find the lock: %v", err)
	}
	if !found.LastSeen().After(alice.LastSeen()) {
		t.Errorf("last seen %s, want it raised by the write", found.LastSeen())
	}
}

// TestDraftUpdate_SceneChangeKeepsItsWidgets: an update writes the Scene's row
// only; its widgets are not removed by the write (a regression of the
// snapshot writer's delete pass).
func TestDraftUpdate_SceneChangeKeepsItsWidgets(t *testing.T) {
	cleanScenes(t)
	cleanEditLocks(t)
	repo := repositories.NewSceneRepositoryPostgres(testDB, mainProject)
	w := makeWidget(t, repo, "w1")
	saved := mustSave(t, repo, withWidgets(makeScene(t, repo, "scene-1"), w))
	if len(saved.Widgets()) != 1 {
		t.Fatalf("the saved scene has %d widgets, want 1", len(saved.Widgets()))
	}
	name, _ := scene.NewSceneName("renamed")
	updated := scene.ReconstituteScene(saved.ID(), saved.ProjectID(), name, saved.Size(), saved.BackgroundHTML(), nil)
	mustUpdate(t, mainProject, "rename scene", func(session *draft.Session) error {
		_, err := session.UpdateScene(context.Background(), saved.ID(), updated.Name(), updated.Size(), updated.BackgroundHTML())
		return err
	})
	found := mustFindByID(t, repo, saved.ID())
	if found.Name() != name || len(found.Widgets()) != 1 {
		t.Errorf("the renamed scene: %q with %d widgets, want %q with 1", found.Name(), len(found.Widgets()), name)
	}
}

// TestDraft_RowsGoWithTheirScope: a Project removed takes its Draft and its
// DraftChanges along (ON DELETE CASCADE).
func TestDraft_RowsGoWithTheirScope(t *testing.T) {
	cleanProjects(t)
	ctx := editlock.System(context.Background())
	projects := repositories.NewProjectRepositoryPostgres(testDB)
	p := mustCreateProject(t, projects, "Line 1")
	tagRepo := repositories.NewTagRepositoryPostgres(testDB, p.ID())
	name, _ := tag.NewTagName("speed")
	created, err := tag.CreateTag(tagRepo.NextID(), p.ID(), name, tag.TagTypeInteger, tag.NoSupplier())
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	mustSaveTag(t, tagRepo, created)

	var draftID uuid.UUID
	if err := testDB.QueryRowContext(ctx, `SELECT id FROM engineering.drafts WHERE project_id = $1`, p.ID().UUID()).Scan(&draftID); err != nil {
		t.Fatalf("read the draft: %v", err)
	}
	marked, err := projects.Save(ctx, p.Delete())
	if err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	if _, err := testDrafts().Purge(ctx, editlock.ProjectScope(p.ID())); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if err := projects.Delete(ctx, marked); err != nil {
		t.Fatalf("remove the project's row: %v", err)
	}
	var drafts, changes int
	if err := testDB.QueryRowContext(ctx,
		`SELECT (SELECT count(*) FROM engineering.drafts WHERE id = $1),
		        (SELECT count(*) FROM engineering.draft_changes WHERE draft_id = $1)`, draftID,
	).Scan(&drafts, &changes); err != nil {
		t.Fatalf("count the draft's rows: %v", err)
	}
	if drafts != 0 || changes != 0 {
		t.Errorf("after the project went: %d draft rows, %d changes; want none", drafts, changes)
	}
}

// TestDraftPurge_OfADeletedProject: the cleanup empties a Project marked
// deleted, which is not live, and keeps the Project's row for its own delete.
func TestDraftPurge_OfADeletedProject(t *testing.T) {
	cleanProjects(t)
	ctx := editlock.System(context.Background())
	projects := repositories.NewProjectRepositoryPostgres(testDB)
	p := mustCreateProject(t, projects, "Line 1")
	tagRepo := repositories.NewTagRepositoryPostgres(testDB, p.ID())
	name, _ := tag.NewTagName("speed")
	created, err := tag.CreateTag(tagRepo.NextID(), p.ID(), name, tag.TagTypeInteger, tag.NoSupplier())
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	mustSaveTag(t, tagRepo, created)

	marked, err := projects.Save(ctx, p.Delete())
	if err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	if _, err := testDrafts().Purge(ctx, editlock.ProjectScope(p.ID())); err != nil {
		t.Fatalf("purge: %v", err)
	}
	var tags int
	if err := testDB.QueryRowContext(ctx, `SELECT count(*) FROM engineering.tags WHERE project_id = $1`, p.ID().UUID()).Scan(&tags); err != nil || tags != 0 {
		t.Errorf("tags after the purge: %d, %v", tags, err)
	}
	if err := projects.Delete(ctx, marked); err != nil {
		t.Fatalf("remove the project's row: %v", err)
	}
}

// mustDevice builds a new Device of the project Main, with a name of its own.
func mustDevice(t *testing.T, name string) device.Device {
	t.Helper()
	deviceName, err := device.NewDeviceName(name + "-" + uuid.NewString())
	if err != nil {
		t.Fatalf("NewDeviceName: %v", err)
	}
	config, err := device.NewDeviceConfig([]byte(`{}`))
	if err != nil {
		t.Fatalf("NewDeviceConfig: %v", err)
	}
	created, err := device.CreateDevice(id.NewID[device.Device](), mainProject, deviceName, "", device.ProtocolSimulation, config)
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	return created
}

// TestDraftReplace_RemovesThePairsTheRevisionLacks: a pair (Environment,
// Device) has no ID of its own — its Environment's ID and its Device's ID
// together name it. A Discard (Replace) drops the pairs the Revision does not
// hold even while their Environment stays.
func TestDraftReplace_RemovesThePairsTheRevisionLacks(t *testing.T) {
	cleanEditLocks(t)
	ctx := editlock.System(context.Background())
	if _, err := testDB.ExecContext(ctx,
		`DELETE FROM engineering.environment_devices; DELETE FROM engineering.devices; DELETE FROM engineering.environments`); err != nil {
		t.Fatalf("clean devices: %v", err)
	}

	environmentName, err := device.NewEnvironmentName("bench-" + uuid.NewString())
	if err != nil {
		t.Fatalf("NewEnvironmentName: %v", err)
	}
	environment, err := device.CreateEnvironment(id.NewID[device.Environment](), mainProject, environmentName, "")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	one, two := mustDevice(t, "one"), mustDevice(t, "two")
	params, err := device.NewEnvironmentDeviceParams([]byte(`{"address":"127.0.0.1:1502"}`))
	if err != nil {
		t.Fatalf("NewEnvironmentDeviceParams: %v", err)
	}
	kept := device.CreateEnvironmentDevice(mainProject, environment.ID(), one.ID(), params)

	mustUpdate(t, mainProject, "devices and environment", func(session *draft.Session) error {
		session.CreateDevice(ctx, one)
		session.CreateDevice(ctx, two)
		session.CreateEnvironment(ctx, environment)
		return nil
	})
	mustUpdate(t, mainProject, "pairs", func(session *draft.Session) error {
		if _, _, err := session.PutEnvironmentDevice(ctx, environment.ID(), one.ID(), params); err != nil {
			return err
		}
		_, _, err := session.PutEnvironmentDevice(ctx, environment.ID(), two.ID(), params)
		return err
	})

	// The Revision the Discard restores holds the Environment with one pair.
	if _, err := testDrafts().Replace(ctx, editlock.ProjectScope(mainProject), func(session *draft.Session) error {
		return session.Replace(ctx, draft.Content{
			Devices:            []device.Device{one, two},
			Environments:       []device.Environment{environment},
			EnvironmentDevices: []device.EnvironmentDevice{kept},
		})
	}); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	pairs, err := repositories.NewEnvironmentDeviceRepositoryPostgres(testDB, mainProject).FindByEnvironment(ctx, environment.ID())
	if err != nil {
		t.Fatalf("FindByEnvironment: %v", err)
	}
	if len(pairs) != 1 || pairs[0].DeviceID() != one.ID() {
		t.Errorf("the pairs after the Discard: %+v, want only the Device %s", pairs, one.ID())
	}
}
