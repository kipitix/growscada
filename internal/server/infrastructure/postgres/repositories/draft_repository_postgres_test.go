package repositories_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/draft"
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
	cleanDrafts(t)
	ctx := context.Background()
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
	_, err := testDrafts().Update(ctx, draft.ProjectScope(mainProject), first, "create late tag",
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

// TestDraftCheckVersion: the check around a multi-step act over the Draft
// passes at the Version it read and is a draft_conflict once a write raised it
// (ADR 0022).
func TestDraftCheckVersion(t *testing.T) {
	cleanTags(t)
	cleanDrafts(t)
	ctx := context.Background()
	scope := draft.ProjectScope(mainProject)
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	read := draftVersionOf(t, mainProject)

	if err := testDrafts().CheckVersion(ctx, scope, read); err != nil {
		t.Fatalf("CheckVersion at the current version: %v", err)
	}

	created := makeTag(t, "speed", repo)
	mustUpdate(t, mainProject, "Create tag speed", func(session *draft.Session) error {
		session.CreateTag(ctx, created)
		return nil
	})
	if err := testDrafts().CheckVersion(ctx, scope, read); !errors.Is(err, draft.ErrDraftConflict) {
		t.Fatalf("CheckVersion with a stale version: %v, want ErrDraftConflict", err)
	}
	if err := testDrafts().CheckVersion(ctx, scope, draftVersionOf(t, mainProject)); err != nil {
		t.Errorf("CheckVersion at the new version: %v", err)
	}
}

// TestDraftCleanVersion_SaveDiscardAndNothingToSave: the Draft's Clean Version
// is the marker of its unsaved changes and of the Save's refusal — a write is
// unsaved, a Save cleans it, a clean replacement cleans it, an unclean one does
// not (task 74; ADR 0016, amendment 2026-11-10).
func TestDraftCleanVersion_SaveDiscardAndNothingToSave(t *testing.T) {
	cleanTags(t)
	cleanScenes(t)
	cleanDrafts(t)
	ctx := context.Background()
	scope := draft.ProjectScope(mainProject)
	tags := repositories.NewTagRepositoryPostgres(testDB, mainProject)

	read := func() draft.Draft {
		t.Helper()
		found, ok, err := testDrafts().Read(ctx, scope)
		if err != nil || !ok {
			t.Fatalf("read draft: %+v, %t, %v", found, ok, err)
		}
		return found
	}
	if _, ok, err := testDrafts().Read(ctx, scope); err != nil || ok {
		t.Fatalf("a Draft never written has a row: %v", err)
	}

	// A write of the content raises the Version above the marker: unsaved.
	mustUpdate(t, mainProject, "create a tag", func(session *draft.Session) error {
		session.CreateTag(ctx, makeTag(t, "clean-speed", tags))
		return nil
	})
	if !read().HasUnsavedChanges() {
		t.Fatalf("a write above the Clean Version is clean")
	}

	// A Save makes the Draft its Commit's content, and nothing is left to save.
	if _, err := testDrafts().SaveCommit(ctx, scope, draftVersionOf(t, mainProject),
		commitTitle(t, "First"), commitAuthor(t), projectCommitContent()); err != nil {
		t.Fatalf("SaveCommit: %v", err)
	}
	saved := read()
	if saved.HasUnsavedChanges() || saved.CleanVersion() != saved.Version() {
		t.Errorf("a saved Draft: %s, clean %s; want it clean", saved.Version(), saved.CleanVersion())
	}
	if _, err := testDrafts().SaveCommit(ctx, scope, draftVersionOf(t, mainProject),
		commitTitle(t, "Again"), commitAuthor(t), projectCommitContent()); !errors.Is(err, draft.ErrNothingToSave) {
		t.Fatalf("a Save at the Clean Version: %v, want ErrNothingToSave", err)
	}

	// A replacement whose content is the Commit's — a Discard — is clean, and a
	// Save of it is refused too.
	result, err := testDrafts().Replace(ctx, scope, true, func(session *draft.Session) error {
		return session.Replace(ctx, draft.Content{})
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if result.HasUnsavedChanges() {
		t.Errorf("a clean replacement is unsaved: %s, clean %s", result.Version(), result.CleanVersion())
	}
	if _, err := testDrafts().SaveCommit(ctx, scope, draftVersionOf(t, mainProject),
		commitTitle(t, "After a discard"), commitAuthor(t), projectCommitContent()); !errors.Is(err, draft.ErrNothingToSave) {
		t.Fatalf("a Save after a Discard: %v, want ErrNothingToSave", err)
	}

	// A replacement whose content is another one keeps the marker: the raised
	// Version makes the Draft unsaved again.
	if _, err := testDrafts().Replace(ctx, scope, false, func(session *draft.Session) error {
		return session.Replace(ctx, draft.Content{})
	}); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if !read().HasUnsavedChanges() {
		t.Errorf("a replacement that is not the Commit's is clean")
	}
}

// TestDraftUpdate_WritesOneDraftChange: every successful write stores one row
// with the before and after states of what it touched, and moves the stack
// position (ADR 0017).
func TestDraftUpdate_WritesOneDraftChange(t *testing.T) {
	cleanTags(t)
	cleanScenes(t)
	cleanDrafts(t)
	ctx := context.Background()
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

// TestDraftOpen_OneChangeWithOnlyTheDifference: opening a project file writes
// one DraftChange with the entities that differ, raises the Version, drops the
// redo tail, and undo restores the previous Draft (task 39, ADR 0018, ADR 0017).
func TestDraftOpen_OneChangeWithOnlyTheDifference(t *testing.T) {
	cleanTags(t)
	cleanScenes(t)
	cleanDrafts(t)
	ctx := context.Background()
	scope := draft.ProjectScope(mainProject)
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	speed := makeTag(t, "speed", repo)
	pressure := makeTag(t, "pressure", repo)

	mustUpdate(t, mainProject, "Create speed", func(session *draft.Session) error {
		session.CreateTag(ctx, speed)
		return nil
	})
	mustUpdate(t, mainProject, "Create pressure", func(session *draft.Session) error {
		session.CreateTag(ctx, pressure)
		return nil
	})
	// Undo the second change: a redo tail the open must drop.
	if _, err := testDrafts().Undo(ctx, scope, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("undo: %v", err)
	}

	renamedSpeed, err := tag.NewTagName("speed-2")
	if err != nil {
		t.Fatalf("tag name: %v", err)
	}
	renamed, err := tag.ReconstituteTag(speed.ID(), mainProject, renamedSpeed, speed.Type(), tag.NoSupplier())
	if err != nil {
		t.Fatalf("reconstitute the renamed tag: %v", err)
	}
	flowName, err := tag.NewTagName("flow")
	if err != nil {
		t.Fatalf("tag name: %v", err)
	}
	flow, err := tag.ReconstituteTag(repo.NextID(), mainProject, flowName, tag.TagTypeInteger, tag.NoSupplier())
	if err != nil {
		t.Fatalf("reconstitute the new tag: %v", err)
	}

	before := draftVersionOf(t, mainProject).Number()
	mustUpdate(t, mainProject, "Open a project file", func(session *draft.Session) error {
		return session.Open(ctx, draft.Content{Tags: []tag.Tag{renamed, flow}})
	})
	if got := draftVersionOf(t, mainProject).Number(); got != before+1 {
		t.Fatalf("version after open %d, want %d", got, before+1)
	}

	var (
		count       int
		description string
	)
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*), max(description) FROM engineering.draft_changes dc
		 JOIN engineering.drafts d ON d.id = dc.draft_id WHERE d.project_id = $1`,
		mainProject.UUID()).Scan(&count, &description); err != nil {
		t.Fatalf("read draft changes: %v", err)
	}
	if count != 2 || description != "Open a project file" {
		t.Errorf("draft changes: %d rows, last %q; want the open as one change and the redo tail dropped", count, description)
	}
	state, ok, err := testDrafts().Stack(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("stack: %+v, %t, %v", state, ok, err)
	}
	if state.Undo == nil || state.Undo.Description != "Open a project file" || state.Redo != nil {
		t.Errorf("stack after open: undo %+v, redo %+v", state.Undo, state.Redo)
	}

	storedSpeed, err := repo.FindByID(ctx, speed.ID())
	if err != nil || storedSpeed.Name().String() != "speed-2" {
		t.Errorf("the renamed tag: %+v, %v", storedSpeed, err)
	}
	if _, err := repo.FindByName(ctx, flowName); err != nil {
		t.Errorf("the added tag: %v", err)
	}

	// Undo restores the previous Draft: the old name and no new Tag.
	if _, err := testDrafts().Undo(ctx, scope, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("undo the open: %v", err)
	}
	storedSpeed, err = repo.FindByID(ctx, speed.ID())
	if err != nil || storedSpeed.Name().String() != "speed" {
		t.Errorf("after undo the tag is %+v, %v; want speed", storedSpeed, err)
	}
	if _, err := repo.FindByName(ctx, flowName); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("the tag of the undone open is still stored: %v", err)
	}
}

// TestDraftUpdate_SceneChangeKeepsItsWidgets: an update writes the Scene's row
// only; its widgets are not removed by the write (a regression of the
// snapshot writer's delete pass).
func TestDraftUpdate_SceneChangeKeepsItsWidgets(t *testing.T) {
	cleanScenes(t)
	cleanDrafts(t)
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
	ctx := context.Background()
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
	if _, err := testDrafts().Purge(ctx, draft.ProjectScope(p.ID())); err != nil {
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
	ctx := context.Background()
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
	if _, err := testDrafts().Purge(ctx, draft.ProjectScope(p.ID())); err != nil {
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

// TestDraftReplace_RemovesThePairsTheDeploymentLacks: a pair (Environment,
// Device) has no ID of its own — its Environment's ID and its Device's ID
// together name it. A Discard (Replace) drops the pairs the Deployment does not
// hold even while their Environment stays.
func TestDraftReplace_RemovesThePairsTheDeploymentLacks(t *testing.T) {
	cleanDrafts(t)
	ctx := context.Background()
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

	// The Deployment the Discard restores holds the Environment with one pair.
	if _, err := testDrafts().Replace(ctx, draft.ProjectScope(mainProject), false, func(session *draft.Session) error {
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
	cleanDrafts(t)
	cleanWidgetTypes(t)
	cleanScenes(t)
	cleanTags(t)
	ctx := context.Background()

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
	cleanDrafts(t)
	cleanWidgetTypes(t)
	cleanScenes(t)
	cleanTags(t)
	ctx := context.Background()

	typeRepo := newWidgetTypeRepo(t)
	portName, err := library.NewInputPortName("value")
	if err != nil {
		t.Fatalf("NewInputPortName: %v", err)
	}
	built := mustCreateWidgetType(t, typeRepo.libraryID, makeWidgetType(t, "gauge", []library.InputPort{
		library.NewInputPort(portName, "", library.AnyTypeHint()),
	}, typeRepo))
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
	narrowed, err := current.Update(current.Name(), current.HtmlTemplate(),
		current.Script(), current.ScriptLanguage(), current.DefaultSize(), nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	mustUpdateLibraryWidgetType(t, typeRepo.libraryID, narrowed)
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

// lastLibraryDraftChange returns the description and the decoded before and
// after states of the latest DraftChange of a Library.
func lastLibraryDraftChange(t *testing.T, libraryID id.ID[library.Library]) (string, []draft.Snapshot, []draft.Snapshot) {
	t.Helper()
	var (
		description           string
		beforeJSON, afterJSON []byte
	)
	err := testDB.QueryRow(
		`SELECT dc.description, dc.before, dc.after
		 FROM engineering.draft_changes dc JOIN engineering.drafts d ON d.id = dc.draft_id
		 WHERE d.library_id = $1 ORDER BY dc.seq DESC LIMIT 1`, libraryID.UUID()).
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

// libraryDraftChangeCount returns how many DraftChanges the Library's Draft
// holds.
func libraryDraftChangeCount(t *testing.T, libraryID id.ID[library.Library]) int {
	t.Helper()
	var count int
	err := testDB.QueryRow(
		`SELECT count(*) FROM engineering.draft_changes dc JOIN engineering.drafts d ON d.id = dc.draft_id
		 WHERE d.library_id = $1`, libraryID.UUID()).Scan(&count)
	if err != nil {
		t.Fatalf("count the draft changes: %v", err)
	}
	return count
}

// TestDraftLibrary_OneChangePerWriteAndCAS: a write of a Library's Draft
// raises its one Version and appends one DraftChange; a stale Version is a
// conflict that writes nothing (ADR 0016, ADR 0017).
func TestDraftLibrary_OneChangePerWriteAndCAS(t *testing.T) {
	cleanWidgetTypes(t)
	ctx := context.Background()
	repo := newWidgetTypeRepo(t)
	scope := draft.LibraryScope(repo.libraryID)
	initial := libraryDraftVersionOf(t, repo.libraryID)
	wt := makeWidgetType(t, "gauge", nil, repo)

	created, err := testDrafts().Update(ctx, scope, initial, "Create widget type gauge", func(session *draft.Session) error {
		session.CreateWidgetType(ctx, wt)
		return nil
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Version() != initial.Next() {
		t.Errorf("version %s after the write, want %s", created.Version(), initial.Next())
	}
	description, before, after := lastLibraryDraftChange(t, repo.libraryID)
	if description != "Create widget type gauge" {
		t.Errorf("the change: %q", description)
	}
	if _, ok := snapshotOf(before, draft.KindWidgetType, wt.ID().UUID()); ok {
		t.Error("the before side of a created type is not empty")
	}
	snapshot, ok := snapshotOf(after, draft.KindWidgetType, wt.ID().UUID())
	if !ok || snapshot.State.(draft.WidgetTypeState).Name != "gauge" {
		t.Errorf("the after side: %+v", snapshot.State)
	}

	again := makeWidgetType(t, "dial", nil, repo)
	_, err = testDrafts().Update(ctx, scope, initial, "Create widget type dial", func(session *draft.Session) error {
		session.CreateWidgetType(ctx, again)
		return nil
	})
	if !errors.Is(err, draft.ErrDraftConflict) {
		t.Fatalf("a stale version: %v, want ErrDraftConflict", err)
	}
	if count := libraryDraftChangeCount(t, repo.libraryID); count != 1 {
		t.Errorf("%d changes after the refused write, want 1", count)
	}
	if version := libraryDraftVersionOf(t, repo.libraryID); version != created.Version() {
		t.Errorf("version %s after the refused write, want %s", version, created.Version())
	}
	if list, err := repo.FindAll(ctx); err != nil || len(list) != 1 {
		t.Errorf("the Draft after the refused write: %v, %v", list, err)
	}
}

// TestDraftLibrary_ReplaceRestoresTheReleaseAndClearsTheStack: a Discard is
// one transaction over the Draft: the released names come back even when two
// were swapped, the Version rises and the DraftChanges go (ADR 0016).
func TestDraftLibrary_ReplaceRestoresTheReleaseAndClearsTheStack(t *testing.T) {
	cleanWidgetTypes(t)
	ctx := context.Background()
	repo := newWidgetTypeRepo(t)
	scope := draft.LibraryScope(repo.libraryID)
	a := mustStoreWidgetType(t, makeWidgetType(t, "a", nil, repo), repo)
	b := mustStoreWidgetType(t, makeWidgetType(t, "b", nil, repo), repo)
	release := mustRelease(t, repo.libraryID)

	rename := func(wt library.WidgetType, name string) {
		t.Helper()
		named, err := library.NewWidgetTypeName(name)
		if err != nil {
			t.Fatal(err)
		}
		changed, err := wt.Update(named, wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.DefaultSize(), wt.InputPorts())
		if err != nil {
			t.Fatal(err)
		}
		mustUpdateLibraryWidgetType(t, repo.libraryID, changed)
	}
	rename(a, "tmp")
	rename(b, "a")
	rename(mustFindWidgetType(t, repo, a.ID()), "b")
	if count := libraryDraftChangeCount(t, repo.libraryID); count != 5 {
		t.Fatalf("%d changes before the Discard, want 5", count)
	}

	before := libraryDraftVersionOf(t, repo.libraryID)
	result, err := testDrafts().Replace(ctx, scope, false, func(session *draft.Session) error {
		return session.ReplaceWidgetTypes(ctx, release.WidgetTypes())
	})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if result.Version() != before.Next() {
		t.Errorf("version %s after the Discard, want %s", result.Version(), before.Next())
	}
	if count := libraryDraftChangeCount(t, repo.libraryID); count != 0 {
		t.Errorf("%d changes left after the Discard, want none", count)
	}
	names := map[uuid.UUID]string{}
	list, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	for _, wt := range list {
		names[wt.ID().UUID()] = wt.Name().String()
	}
	if len(names) != 2 || names[a.ID().UUID()] != "a" || names[b.ID().UUID()] != "b" {
		t.Errorf("the Draft after the Discard: %v", names)
	}
}

// mustFindWidgetType reads a WidgetType of the Library's Draft.
func mustFindWidgetType(t *testing.T, repo libraryRepo, typeID id.ID[library.WidgetType]) library.WidgetType {
	t.Helper()
	found, err := repo.FindByID(context.Background(), typeID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	return found
}

// TestDraftUndo_RestoresTheDeletedTagAndItsBindings: the undo of a Tag deletion
// writes the before states of its DraftChange — the Tag and the Widget bound —
// raises the Draft's Version, steps the stack back, publishes the inverse
// events and appends no DraftChange of its own (ADR 0017).
func TestDraftUndo_RestoresTheDeletedTagAndItsBindings(t *testing.T) {
	cleanDrafts(t)
	cleanWidgetTypes(t)
	cleanScenes(t)
	cleanTags(t)
	cleanOutbox(t)
	ctx := context.Background()

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
	cleanOutbox(t)
	changes := draftChangeCount(t, mainProject)
	scope := draft.ProjectScope(mainProject)
	version := draftVersionOf(t, mainProject)

	stack, ok, err := testDrafts().Stack(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("Stack: %v (found %t)", err, ok)
	}
	if !stack.CanUndo() || stack.CanRedo() || stack.Undo.Description != "Delete tag pressure" {
		t.Fatalf("the stack: undo %v, redo %v, undo %q", stack.CanUndo(), stack.CanRedo(), stack.Undo.Description)
	}
	position := stack.Draft.StackPosition()

	step, err := testDrafts().Undo(ctx, scope, version)
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if step.Version().Number() != version.Number()+1 || step.StackPosition() != position-1 {
		t.Errorf("after the undo: version %d, position %d; want %d, %d",
			step.Version().Number(), step.StackPosition(), version.Number()+1, position-1)
	}
	if _, err := tagRepo.FindByID(ctx, bound.ID()); err != nil {
		t.Errorf("the Tag did not come back: %v", err)
	}
	found := mustFindByID(t, sceneRepo, saved.ID())
	if len(found.Widgets()) != 1 || len(found.Widgets()[0].PortBindings()) != 1 {
		t.Errorf("the Scene after the undo: %+v, want the Widget bound again", found.Widgets())
	}
	if got := outboxTypes(t); !slices.Equal(got, []string{"tag_created", "widget_updated"}) {
		t.Errorf("the undo published %v, want tag_created and widget_updated", got)
	}
	if after := draftChangeCount(t, mainProject); after != changes {
		t.Errorf("the undo wrote %d DraftChanges, want none (had %d)", after-changes, changes)
	}
}

// TestDraftUndoRedo_RedoReappliesAndANewChangeTrimsTheTail: a redo writes the
// after states again, and a new change after an undo deletes the redo tail, so
// the redo has nothing left to apply (ADR 0017).
func TestDraftUndoRedo_RedoReappliesAndANewChangeTrimsTheTail(t *testing.T) {
	cleanDrafts(t)
	cleanTags(t)
	cleanOutbox(t)
	ctx := context.Background()
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	scope := draft.ProjectScope(mainProject)

	first := makeTag(t, "speed", repo)
	mustSaveTag(t, repo, first)
	second := makeTag(t, "pressure", repo)
	mustSaveTag(t, repo, second)

	if _, err := testDrafts().Undo(ctx, scope, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if _, err := repo.FindByID(ctx, second.ID()); err == nil {
		t.Errorf("the undone Tag is still in the Draft")
	}
	// A new change after the undo removes the redo tail.
	third := makeTag(t, "temperature", repo)
	mustSaveTag(t, repo, third)
	if _, err := testDrafts().Redo(ctx, scope, draftVersionOf(t, mainProject)); !errors.Is(err, draft.ErrNothingToRedo) {
		t.Fatalf("Redo after a new change: %v, want ErrNothingToRedo", err)
	}
	if _, err := repo.FindByID(ctx, second.ID()); err == nil {
		t.Errorf("the trimmed redo brought its Tag back")
	}
}

// TestDraftUndo_EmptyStackAndStaleVersion: an undo or redo with nothing to
// apply is refused, and so is one that states a stale Version.
func TestDraftUndo_EmptyStackAndStaleVersion(t *testing.T) {
	cleanDrafts(t)
	cleanTags(t)
	ctx := context.Background()
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	scope := draft.ProjectScope(mainProject)
	empty := draftVersionOf(t, mainProject)

	if _, err := testDrafts().Undo(ctx, scope, empty); !errors.Is(err, draft.ErrNothingToUndo) {
		t.Errorf("undo of an empty stack: %v, want ErrNothingToUndo", err)
	}
	if _, err := testDrafts().Redo(ctx, scope, empty); !errors.Is(err, draft.ErrNothingToRedo) {
		t.Errorf("redo of an empty stack: %v, want ErrNothingToRedo", err)
	}

	created := makeTag(t, "speed", repo)
	mustSaveTag(t, repo, created)
	if _, err := testDrafts().Undo(ctx, scope, empty); !errors.Is(err, draft.ErrDraftConflict) {
		t.Errorf("undo with a stale version: %v, want ErrDraftConflict", err)
	}
	if _, err := testDrafts().Undo(ctx, scope, draftVersionOf(t, mainProject)); err != nil {
		t.Errorf("undo: %v", err)
	}
}

// TestDraftUndo_LibraryWidgetType: a Library's Draft undoes the creation of a
// WidgetType and redoes it, with the events of each side.
func TestDraftUndo_LibraryWidgetType(t *testing.T) {
	cleanWidgetTypes(t)
	cleanDrafts(t)
	cleanOutbox(t)
	ctx := context.Background()
	repo := newWidgetTypeRepo(t)
	scope := draft.LibraryScope(repo.libraryID)
	saved := mustCreateWidgetType(t, repo.libraryID, makeWidgetType(t, "gauge", nil, repo))
	cleanOutbox(t)

	if _, err := testDrafts().Undo(ctx, scope, libraryDraftVersionOf(t, repo.libraryID)); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if _, err := repo.FindByID(ctx, saved.ID()); !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("the undone WidgetType is still in the Draft: %v", err)
	}
	if got := outboxTypes(t); !slices.Equal(got, []string{"widget_type_deleted"}) {
		t.Errorf("the undo published %v, want widget_type_deleted", got)
	}

	cleanOutbox(t)
	if _, err := testDrafts().Redo(ctx, scope, libraryDraftVersionOf(t, repo.libraryID)); err != nil {
		t.Fatalf("Redo: %v", err)
	}
	if _, err := repo.FindByID(ctx, saved.ID()); err != nil {
		t.Errorf("the redone WidgetType did not come back: %v", err)
	}
	if got := outboxTypes(t); !slices.Equal(got, []string{"widget_type_created"}) {
		t.Errorf("the redo published %v, want widget_type_created", got)
	}
}

// TestDraftChanges_KeepTheLatestHundred: the stack is trimmed to its depth,
// older changes being dropped and no longer undoable (ADR 0017).
func TestDraftChanges_KeepTheLatestHundred(t *testing.T) {
	cleanDrafts(t)
	cleanTags(t)
	repo := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	for i := 0; i < 101; i++ {
		mustSaveTag(t, repo, makeTag(t, "tag-"+uuid.NewString(), repo))
	}
	var count, minSeq int
	if err := testDB.QueryRow(
		`SELECT count(*), min(dc.seq) FROM engineering.draft_changes dc
		 JOIN engineering.drafts d ON d.id = dc.draft_id WHERE d.project_id = $1`, mainProject.UUID(),
	).Scan(&count, &minSeq); err != nil {
		t.Fatalf("read the stack: %v", err)
	}
	if count != 100 || minSeq != 2 {
		t.Errorf("the stack holds %d changes from %d, want 100 from 2", count, minSeq)
	}
}

// draftChangeCount is how many DraftChanges the Project's Draft holds.
func draftChangeCount(t *testing.T, projectID id.ID[project.Project]) int {
	t.Helper()
	var count int
	if err := testDB.QueryRow(
		`SELECT count(*) FROM engineering.draft_changes dc
		 JOIN engineering.drafts d ON d.id = dc.draft_id WHERE d.project_id = $1`, projectID.UUID(),
	).Scan(&count); err != nil {
		t.Fatalf("count the draft changes: %v", err)
	}
	return count
}

// TestDraftUndo_RestoresADeletedSceneWithItsWidgets: the snapshots of a removed
// Scene carry each of its Widgets, and the undo writes them all back — the
// rows of the intent, applied together (ADR 0017).
func TestDraftUndo_RestoresADeletedSceneWithItsWidgets(t *testing.T) {
	cleanDrafts(t)
	cleanWidgetTypes(t)
	cleanScenes(t)
	ctx := context.Background()
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB, mainProject)
	first := makeWidget(t, sceneRepo, "w1")
	second := makeWidget(t, sceneRepo, "w2")
	saved := mustSave(t, sceneRepo, withWidgets(makeScene(t, sceneRepo, "main"), first, second))

	if _, err := testDrafts().Update(ctx, draft.ProjectScope(mainProject), draftVersionOf(t, mainProject),
		"Delete scene main", func(session *draft.Session) error {
			_, err := session.DeleteScene(ctx, saved.ID())
			return err
		}); err != nil {
		t.Fatalf("delete the scene: %v", err)
	}
	if _, err := sceneRepo.FindByID(ctx, saved.ID()); !errors.Is(err, scene.ErrSceneNotFound) {
		t.Fatalf("the scene is still there: %v", err)
	}

	if _, err := testDrafts().Undo(ctx, draft.ProjectScope(mainProject), draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	found := mustFindByID(t, sceneRepo, saved.ID())
	if len(found.Widgets()) != 2 {
		t.Errorf("the restored scene holds %d widgets, want its 2", len(found.Widgets()))
	}
}

// TestDraftMoveWidgets_OneChangeWithBothPositions: a group move is one
// DraftChange carrying the before and after state of each widget that moved,
// and its undo puts every position back in one step, publishing widget_updated
// for each (ADR 0010, ADR 0017). A widget that did not move stays out of the
// change.
func TestDraftMoveWidgets_OneChangeWithBothPositions(t *testing.T) {
	cleanDrafts(t)
	cleanWidgetTypes(t)
	cleanScenes(t)
	cleanOutbox(t)
	ctx := context.Background()
	repo := repositories.NewSceneRepositoryPostgres(testDB, mainProject)
	first := makeWidget(t, repo, "w1")
	second := makeWidget(t, repo, "w2")
	staying := makeWidget(t, repo, "w3")
	saved := mustSave(t, repo, withWidgets(makeScene(t, repo, "main"), first, second, staying))
	changes := draftChangeCount(t, mainProject)
	version := draftVersionOf(t, mainProject)

	mustUpdate(t, mainProject, "Move 2 widgets", func(session *draft.Session) error {
		_, err := session.MoveWidgets(ctx, saved.ID(), []scene.WidgetMove{
			scene.NewWidgetMove(first.ID(), 100, 200),
			scene.NewWidgetMove(second.ID(), 300, 400),
		})
		return err
	})
	cleanOutbox(t)

	if got := draftChangeCount(t, mainProject); got != changes+1 {
		t.Errorf("the move wrote %d DraftChanges, want one", got-changes)
	}
	if draftVersionOf(t, mainProject).Number() != version.Number()+1 {
		t.Errorf("the move did not raise the Version once")
	}
	description, before, after := lastDraftChange(t, mainProject)
	if description != "Move 2 widgets" {
		t.Errorf("the change: %q", description)
	}
	if len(before) != 2 || len(after) != 2 {
		t.Errorf("the change holds %d before and %d after snapshots, want 2 and 2", len(before), len(after))
	}
	for _, w := range []scene.Widget{first, second} {
		old, ok := snapshotOf(before, draft.KindWidget, w.ID().UUID())
		if !ok {
			t.Fatalf("no before state of %s", w.ID())
		}
		if got := old.State.(draft.WidgetState); got.X != w.Position().X() || got.Y != w.Position().Y() {
			t.Errorf("before %s: (%v, %v), want (%v, %v)", w.ID(), got.X, got.Y, w.Position().X(), w.Position().Y())
		}
		moved, ok := snapshotOf(after, draft.KindWidget, w.ID().UUID())
		if !ok {
			t.Fatalf("no after state of %s", w.ID())
		}
		if moved.State.(draft.WidgetState).X == w.Position().X() {
			t.Errorf("after %s is where it was", w.ID())
		}
	}
	if _, ok := snapshotOf(after, draft.KindWidget, staying.ID().UUID()); ok {
		t.Error("the widget that did not move is in the change")
	}

	if _, err := testDrafts().Undo(ctx, draft.ProjectScope(mainProject), draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	restored := mustFindByID(t, repo, saved.ID())
	if len(restored.Widgets()) != 3 {
		t.Fatalf("the scene after the undo holds %d widgets, want 3", len(restored.Widgets()))
	}
	want := scene.NewPosition(10, 20, 0)
	for _, w := range restored.Widgets() {
		if w.Position() != want {
			t.Errorf("widget %s after the undo: %s, want %s", w.ID(), w.Position(), want)
		}
	}
	if got := outboxTypes(t); !slices.Equal(got, []string{"widget_updated", "widget_updated"}) {
		t.Errorf("the undo published %v, want widget_updated for each moved widget", got)
	}
}
