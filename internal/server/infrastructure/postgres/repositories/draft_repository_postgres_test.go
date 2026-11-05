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
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/draftchange"
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

// snapshotOf returns the snapshot of kind and ID on one side of a change.
func snapshotOf(list []draft.Snapshot, kind draft.Kind, anID uuid.UUID) (draft.Snapshot, bool) {
	for _, snapshot := range list {
		if snapshot.Kind == kind && snapshot.ID == anID {
			return snapshot, true
		}
	}
	return draft.Snapshot{}, false
}

// lastDraftChange returns the description and the decoded before and after
// states of the latest DraftChange of a Project.
func lastDraftChange(t *testing.T, projectID id.ID[project.Project]) (string, []draft.Snapshot, []draft.Snapshot) {
	t.Helper()
	var (
		description           string
		beforeJSON, afterJSON []byte
	)
	err := testDB.QueryRow(
		`SELECT dc.description, dc.before, dc.after
		 FROM engineering.draft_changes dc JOIN engineering.drafts d ON d.id = dc.draft_id
		 WHERE d.project_id = $1 ORDER BY dc.seq DESC LIMIT 1`, projectID.UUID()).
		Scan(&description, &beforeJSON, &afterJSON)
	if err != nil {
		t.Fatalf("read the last draft change: %v", err)
	}
	before, err := draftchange.Decode(beforeJSON)
	if err != nil {
		t.Fatalf("decode the before of the last draft change: %v", err)
	}
	after, err := draftchange.Decode(afterJSON)
	if err != nil {
		t.Fatalf("decode the after of the last draft change: %v", err)
	}
	return description, before, after
}

// mustWidgetOf builds a Widget of the released type, bound by each port to the
// Tag.
func mustWidgetOf(t *testing.T, repo scene.SceneRepository, released library.ReleasedWidgetType, bindings map[string]tag.Tag) scene.Widget {
	t.Helper()
	name, err := scene.NewWidgetName("w-" + uuid.NewString())
	if err != nil {
		t.Fatalf("NewWidgetName: %v", err)
	}
	ports := make([]scene.PortBinding, 0, len(bindings))
	for port, bound := range bindings {
		portName, err := library.NewInputPortName(port)
		if err != nil {
			t.Fatalf("NewInputPortName(%q): %v", port, err)
		}
		ports = append(ports, scene.NewPortBinding(portName, bound.ID()))
	}
	return scene.NewWidget(repo.NextWidgetID(), name, scene.NewPosition(10, 20, 0),
		library.DefaultSize(), scene.DefaultOrigin(), scene.DefaultRotation(),
		released.ID(), nil, ports)
}

// TestDraftDeleteTag_UnbindsItsWidgetsInOneChange: deleting a Tag removes its
// PortBindings from every Scene in the same DraftChange, whose before side
// carries the Tag and the Widget bound and whose after side the Widget alone,
// unbound (ADR 0010, ADR 0017).
func TestDraftDeleteTag_UnbindsItsWidgetsInOneChange(t *testing.T) {
	cleanEditLocks(t)
	cleanWidgetTypes(t)
	cleanScenes(t)
	cleanTags(t)
	ctx := editlock.System(context.Background())

	tagRepo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	bound := makeTag(t, "pressure", tagRepo)
	mustSaveTag(t, tagRepo, bound)
	portName, err := library.NewInputPortName("value")
	if err != nil {
		t.Fatalf("NewInputPortName: %v", err)
	}
	released := mustPinWidgetType(t, mainProject, library.NewInputPort(portName, "", library.AnyTypeHint()))
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB, mainProject)
	widget := mustWidgetOf(t, sceneRepo, released, map[string]tag.Tag{"value": bound})
	saved := mustSave(t, sceneRepo, withWidgets(makeScene(t, sceneRepo, "main"), widget))

	mustUpdate(t, mainProject, "Delete tag pressure", func(session *draft.Session) error {
		_, err := session.DeleteTag(ctx, bound.ID())
		return err
	})

	found := mustFindByID(t, sceneRepo, saved.ID())
	if len(found.Widgets()) != 1 || len(found.Widgets()[0].PortBindings()) != 0 {
		t.Errorf("the Scene after the deletion: %+v, want the Widget unbound", found.Widgets())
	}
	description, before, after := lastDraftChange(t, mainProject)
	if description != "Delete tag pressure" {
		t.Errorf("the change: %q", description)
	}
	if _, ok := snapshotOf(before, draft.KindTag, bound.ID().UUID()); !ok {
		t.Errorf("the before side lacks the deleted Tag")
	}
	if _, ok := snapshotOf(after, draft.KindTag, bound.ID().UUID()); ok {
		t.Errorf("the after side still holds the deleted Tag")
	}
	old, ok := snapshotOf(before, draft.KindWidget, widget.ID().UUID())
	if !ok || len(old.State.(draft.WidgetState).PortBindings) != 1 {
		t.Errorf("the before side of the Widget: %+v, want its binding", old.State)
	}
	changed, ok := snapshotOf(after, draft.KindWidget, widget.ID().UUID())
	if !ok || len(changed.State.(draft.WidgetState).PortBindings) != 0 {
		t.Errorf("the after side of the Widget: %+v, want it unbound", changed.State)
	}
}

// TestDraftPutLibraryPin_SwitchRemovesBrokenBindingsInOneChange: a switch of
// the pinned Release removes, in its own DraftChange, the PortBindings the new
// Release no longer declares (ADR 0010, ADR 0012, ADR 0017).
func TestDraftPutLibraryPin_SwitchRemovesBrokenBindingsInOneChange(t *testing.T) {
	cleanEditLocks(t)
	cleanWidgetTypes(t)
	cleanScenes(t)
	cleanTags(t)
	ctx := editlock.System(context.Background())

	typeRepo := newWidgetTypeRepo(t)
	portName, err := library.NewInputPortName("value")
	if err != nil {
		t.Fatalf("NewInputPortName: %v", err)
	}
	built, err := typeRepo.Save(ctx, makeWidgetType(t, "gauge", []library.InputPort{
		library.NewInputPort(portName, "", library.AnyTypeHint()),
	}, typeRepo))
	if err != nil {
		t.Fatalf("Save widget type: %v", err)
	}
	first := mustRelease(t, typeRepo.libraryID)
	mustPin(t, mainProject, first)

	tagRepo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	bound := makeTag(t, "speed", tagRepo)
	mustSaveTag(t, tagRepo, bound)
	released, ok := first.FindWidgetType(built.ID())
	if !ok {
		t.Fatalf("the release lacks the type %s", built.ID())
	}
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB, mainProject)
	widget := mustWidgetOf(t, sceneRepo, released, map[string]tag.Tag{"value": bound})
	saved := mustSave(t, sceneRepo, withWidgets(makeScene(t, sceneRepo, "main"), widget))

	// The new Release narrows the type: the port goes.
	current, err := typeRepo.FindByID(ctx, built.ID())
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	narrowed, err := current.Update(current.Version(), current.Name(), current.HtmlTemplate(),
		current.Script(), current.ScriptLanguage(), current.DefaultSize(), nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := typeRepo.Save(ctx, narrowed); err != nil {
		t.Fatalf("Save the narrowed type: %v", err)
	}
	second := mustRelease(t, typeRepo.libraryID)

	mustUpdate(t, mainProject, "Switch pin", func(session *draft.Session) error {
		_, _, err := session.PutLibraryPin(ctx, typeRepo.libraryID, second.Number())
		return err
	})

	found := mustFindByID(t, sceneRepo, saved.ID())
	if len(found.Widgets()) != 1 || len(found.Widgets()[0].PortBindings()) != 0 {
		t.Errorf("the Scene after the switch: %+v, want the Widget unbound", found.Widgets())
	}
	description, before, after := lastDraftChange(t, mainProject)
	if description != "Switch pin" {
		t.Errorf("the change: %q", description)
	}
	old, ok := snapshotOf(before, draft.KindWidget, widget.ID().UUID())
	if !ok || len(old.State.(draft.WidgetState).PortBindings) != 1 {
		t.Errorf("the before side of the Widget: %+v, want its binding", old.State)
	}
	changed, ok := snapshotOf(after, draft.KindWidget, widget.ID().UUID())
	if !ok || len(changed.State.(draft.WidgetState).PortBindings) != 0 {
		t.Errorf("the after side of the Widget: %+v, want it unbound", changed.State)
	}
}
