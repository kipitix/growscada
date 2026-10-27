package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func (r *reconciliation) createProject(t *testing.T, name string) uuid.UUID {
	t.Helper()
	created, err := r.projectSvc.CreateProject(context.Background(), appdto.ProjectInput{Name: name})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return created.ID
}

// countRows returns how many rows of table belong to the project.
func countRows(t *testing.T, table string, projectID uuid.UUID) int {
	t.Helper()
	var n int
	if err := testDB.QueryRowContext(context.Background(), `SELECT count(*) FROM `+table+` WHERE project_id = $1`, projectID).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestProjectService_CreateFindRename(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	svc := r.projectSvc
	ctx := context.Background()

	created, err := svc.CreateProject(ctx, appdto.ProjectInput{Name: "Line 1"})
	if err != nil || created.Name != "Line 1" || created.Version != 1 {
		t.Fatalf("CreateProject: %+v, %v", created, err)
	}
	if _, err := svc.CreateProject(ctx, appdto.ProjectInput{Name: "Line 1"}); !errors.Is(err, project.ErrProjectNameTaken) {
		t.Errorf("a taken name: expected ErrProjectNameTaken, got %v", err)
	}
	if _, err := svc.CreateProject(ctx, appdto.ProjectInput{Name: ""}); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("an empty name: expected ErrInvalidInput, got %v", err)
	}

	byName, err := svc.FindProjectByName(ctx, "Line 1")
	if err != nil || byName.ID != created.ID {
		t.Errorf("FindProjectByName: %+v, %v", byName, err)
	}
	renamed, err := svc.UpdateProject(ctx, created.ID, created.Version, appdto.ProjectInput{Name: "Line 2"})
	if err != nil || renamed.Name != "Line 2" || renamed.Version != 2 {
		t.Fatalf("UpdateProject: %+v, %v", renamed, err)
	}
	if _, err := svc.UpdateProject(ctx, created.ID, created.Version, appdto.ProjectInput{Name: "Line 3"}); !errors.Is(err, project.ErrProjectConflict) {
		t.Errorf("a stale rename: expected ErrProjectConflict, got %v", err)
	}
	if _, err := svc.UpdateProject(ctx, created.ID, renamed.Version, appdto.ProjectInput{Name: "Main"}); !errors.Is(err, project.ErrProjectNameTaken) {
		t.Errorf("a rename to a taken name: expected ErrProjectNameTaken, got %v", err)
	}

	all, err := svc.FindAllProjects(ctx)
	if err != nil || len(all) != 2 || all[0].ID != mainProject || all[1].ID != created.ID {
		t.Errorf("FindAllProjects: %+v, %v", all, err)
	}
}

func TestProjectService_Delete_ProjectIsGoneAtOnceAndItsNameFree(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	p := r.createProject(t, "Line 1")

	deleted, err := r.projectSvc.DeleteProjectByID(ctx, p)
	if err != nil || deleted.ID != p {
		t.Fatalf("DeleteProjectByID: %+v, %v", deleted, err)
	}

	if _, err := r.projectSvc.FindProjectByID(ctx, p); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("FindProjectByID: expected ErrProjectNotFound, got %v", err)
	}
	if _, err := r.tagSvc.FindAllTags(ctx, p); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("tags of a deleted project: expected ErrProjectNotFound, got %v", err)
	}
	if _, err := r.sceneSvc.CreateScene(ctx, p, appdto.SceneInput{Name: "s", Width: 10, Height: 10}); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("a scene in a deleted project: expected ErrProjectNotFound, got %v", err)
	}
	if _, err := r.projectSvc.DeleteProjectByID(ctx, p); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("deleting it again: expected ErrProjectNotFound, got %v", err)
	}
	r.createProject(t, "Line 1")
}

func TestProjectContent_UnknownProject_IsNotFound(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	unknown := uuid.New()

	_, err := r.tagSvc.CreateTag(context.Background(), unknown, appdto.CreateTagInput{Name: "t", Type: "integer"})

	var nf id.NotFoundError
	if !errors.Is(err, project.ErrProjectNotFound) || !errors.As(err, &nf) || nf.ID != unknown {
		t.Errorf("expected ErrProjectNotFound of %s, got %v", unknown, err)
	}
}

func TestProjectContent_ProjectsAreApart(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	other := r.createProject(t, "Line 1")
	input := appdto.CreateTagInput{Name: "speed", Type: "integer"}
	inMain, err := r.tagSvc.CreateTag(ctx, mainProject, input)
	if err != nil {
		t.Fatalf("CreateTag in Main: %v", err)
	}
	inOther, err := r.tagSvc.CreateTag(ctx, other, input)
	if err != nil {
		t.Fatalf("the same name in another project: %v", err)
	}

	if _, err := r.tagSvc.FindTagByID(ctx, mainProject, inOther.ID); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("another project's tag: expected ErrTagNotFound, got %v", err)
	}
	if found, err := r.tagSvc.FindTagByName(ctx, other, "speed"); err != nil || found.ID != inOther.ID {
		t.Errorf("FindTagByName in the other project: %+v, %v", found, err)
	}

	// A Widget binds only a Tag of its Scene's project.
	wt := r.createType(t, "value")
	sc := r.createScene(t, "scene")
	_, err = r.sceneSvc.CreateWidget(ctx, mainProject, sc.ID, sc.Version, appdto.WidgetInput{
		Name: "w", Width: 100, Height: 100, OriginX: 0.5, OriginY: 0.5, TypeID: wt.ID,
		PortBindings: []appdto.PortBinding{{PortName: "value", TagID: inOther.ID}},
	})
	if !errors.Is(err, application.ErrInvalidInput) || !errors.Is(err, scene.ErrBoundTagNotFound) {
		t.Errorf("binding another project's tag: expected ErrInvalidInput wrapping ErrBoundTagNotFound, got %v", err)
	}
	if _, err := r.sceneSvc.FindSceneByID(ctx, other, sc.ID); !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("Main's scene through the other project: expected ErrSceneNotFound, got %v", err)
	}
	_ = inMain
}

func TestProjectCleanupPolicy_JobKey(t *testing.T) {
	policy := application.NewProjectCleanupPolicy(nil, nil, nil)
	want := "project:" + mainProject.String()
	for _, e := range []event.Event{
		project.NewProjectDeletedEvent(mainProjectID),
	} {
		if key, ok := policy.JobKey(e); !ok || key != want {
			t.Errorf("%s: key %q (%t), want %q", e.Type(), key, ok, want)
		}
	}
	for _, e := range []event.Event{
		project.NewProjectCreatedEvent(mainProjectID),
		project.NewProjectUpdatedEvent(mainProjectID),
		tag.NewTagCreatedEvent(id.NewID[tag.Tag](), mainProjectID),
		scene.NewSceneCreatedEvent(id.NewID[scene.Scene](), mainProjectID),
		tag.NewTagDeletedEvent(id.NewID[tag.Tag](), mainProjectID),
		scene.NewSceneDeletedEvent(id.NewID[scene.Scene](), mainProjectID),
	} {
		if key, ok := policy.JobKey(e); ok {
			t.Errorf("%s: unexpected job %q", e.Type(), key)
		}
	}
}

func TestProjectCleanup_RemovesScenesThenTagsThenTheProject(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	p := r.createProject(t, "Line 1")
	wt := r.createTypeIn(t, p, "value")
	sc, err := r.sceneSvc.CreateScene(ctx, p, appdto.SceneInput{Name: "s", Width: 100, Height: 100})
	if err != nil {
		t.Fatalf("CreateScene: %v", err)
	}
	speed, err := r.tagSvc.CreateTag(ctx, p, appdto.CreateTagInput{Name: "speed", Type: "integer"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	w, err := r.sceneSvc.CreateWidget(ctx, p, sc.ID, sc.Version, appdto.WidgetInput{
		Name: "w", Width: 100, Height: 100, OriginX: 0.5, OriginY: 0.5, TypeID: wt.ID,
		PortBindings: []appdto.PortBinding{{PortName: "value", TagID: speed.ID}},
	})
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	mainTag := r.createTag(t, "integer")
	r.settle(t)
	r.takeDelivered()

	if _, err := r.projectSvc.DeleteProjectByID(ctx, p); err != nil {
		t.Fatalf("DeleteProjectByID: %v", err)
	}
	r.settle(t)

	for _, table := range []string{"engineering.tags", "engineering.scenes", "engineering.widgets", "engineering.library_pins", "runtime.tags"} {
		if n := countRows(t, table, p); n != 0 {
			t.Errorf("%s of the deleted project left: %d", table, n)
		}
	}
	var rows int
	if err := testDB.QueryRowContext(ctx, `SELECT count(*) FROM projects WHERE id = $1`, p).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("the project's row: %d, %v", rows, err)
	}
	var types []string
	for _, e := range r.takeDelivered() {
		types = append(types, e.Type().String())
	}
	want := []string{"project_deleted", "widget_deleted", "scene_deleted", "tag_deleted", "library_pin_deleted"}
	if !slices.Equal(types, want) {
		t.Errorf("delivered %v, want %v", types, want)
	}
	if _, err := r.tagSvc.FindTagByID(ctx, mainProject, mainTag); err != nil {
		t.Errorf("Main's tag must stay: %v", err)
	}
	_ = w
	if jobs := jobsOf(t, application.ProjectCleanupPolicyName); len(jobs) != 0 {
		t.Errorf("jobs left: %+v", jobs)
	}
}

func TestProjectCleanup_TagSavedAfterTheProjectIsRemoved_IsProjectNotFound(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	p := r.createProject(t, "Line 1")
	if _, err := r.projectSvc.DeleteProjectByID(ctx, p); err != nil {
		t.Fatalf("DeleteProjectByID: %v", err)
	}
	r.drain(t)
	r.runDue(t) // the project, empty, is removed at once

	// A Tag created by a service that read the project as live before it was
	// marked, written once the cleanup has already run: refused.
	r2 := newScopes().ForProject(id.NewID(id.IDWithUUID[project.Project](p)))
	name, _ := tag.NewTagName("late")
	late, _ := tag.CreateTag(r2.Tags().NextID(), id.NewID(id.IDWithUUID[project.Project](p)), name, tag.TagTypeInteger)
	if _, err := r2.Tags().Save(ctx, late); !errors.Is(err, project.ErrProjectNotFound) {
		t.Fatalf("a tag of a removed project: expected ErrProjectNotFound, got %v", err)
	}
}

// lateTag is a Tag of the project p as a service that read p as live
// before it was marked builds it, with the repository of p's content.
func lateTag(t *testing.T, p uuid.UUID) (tag.TagRepository, tag.Tag) {
	t.Helper()
	projectID := id.NewID(id.IDWithUUID[project.Project](p))
	repo := newScopes().ForProject(projectID).Tags()
	name, _ := tag.NewTagName("late")
	late, err := tag.CreateTag(repo.NextID(), projectID, name, tag.TagTypeInteger)
	if err != nil {
		t.Fatal(err)
	}
	return repo, late
}

func TestProjectCleanup_TagSavedAfterTheProjectIsMarked_IsProjectNotFound(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	p := r.createProject(t, "Line 1")
	// The project is marked deleted; its cleanup job is not run yet.
	if _, err := r.projectSvc.DeleteProjectByID(ctx, p); err != nil {
		t.Fatalf("DeleteProjectByID: %v", err)
	}

	repo, late := lateTag(t, p)
	if _, err := repo.Save(ctx, late); !errors.Is(err, project.ErrProjectNotFound) {
		t.Fatalf("a tag of a marked project: expected ErrProjectNotFound, got %v", err)
	}
	if n := countRows(t, "engineering.tags", p); n != 0 {
		t.Errorf("tags of the marked project: %d", n)
	}
}

func TestProjectCleanup_TagSavedWhileTheProjectIsBeingMarked_WaitsAndIsProjectNotFound(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	p := r.createProject(t, "Line 1")
	// The transaction marking the project holds its row, not committed yet.
	tx, err := testDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET deleted_at = now(), version = version + 1 WHERE id = $1`, p); err != nil {
		t.Fatalf("mark: %v", err)
	}

	repo, late := lateTag(t, p)
	saved := make(chan error, 1)
	go func() {
		_, err := repo.Save(ctx, late)
		saved <- err
	}()
	select {
	case err := <-saved:
		t.Fatalf("the save did not wait for the mark: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit the mark: %v", err)
	}

	if err := <-saved; !errors.Is(err, project.ErrProjectNotFound) {
		t.Fatalf("a tag saved while the project was being marked: expected ErrProjectNotFound, got %v", err)
	}
	if n := countRows(t, "engineering.tags", p); n != 0 {
		t.Errorf("tags of the marked project: %d", n)
	}
}

func TestSetTagValue_TagOfADeletedProject_IsProjectNotFound(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	p := r.createProject(t, "Line 1")
	speed, err := r.tagSvc.CreateTag(ctx, p, appdto.CreateTagInput{Name: "speed", Type: "integer"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if _, err := r.projectSvc.DeleteProjectByID(ctx, p); err != nil {
		t.Fatalf("DeleteProjectByID: %v", err)
	}

	// The cleanup has not run: the tag's state is still there.
	_, err = r.states.SetTagValue(ctx, p, runtimeapp.SetTagValueInput{TagID: speed.ID, Value: "1", Quality: "good"})
	if !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("a value of a deleted project's tag: expected ErrProjectNotFound, got %v", err)
	}
	if _, err := r.states.SetTagValue(ctx, mainProject, runtimeapp.SetTagValueInput{TagID: speed.ID, Value: "1", Quality: "good"}); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("the tag through a live project of its own: expected ErrTagNotFound, got %v", err)
	}
}

// TestProjectCleanup_StateUpsertedAfterTheStatesAreRemoved_IsRemovedOnTheNextRun:
// a Tag state upserted after the cleanup removed the Project's states (a
// creation's call or a bridge job running late) holds the Project's row; the
// cleanup fails, and its next run removes the state, then the row.
func TestProjectCleanup_StateUpsertedAfterTheStatesAreRemoved_IsRemovedOnTheNextRun(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	p := r.createProject(t, "Line 1")
	if _, err := r.projectSvc.DeleteProjectByID(ctx, p); err != nil {
		t.Fatalf("DeleteProjectByID: %v", err)
	}
	late := uuid.New()
	if err := newRuntimeNode().UpsertTagDefinitions(ctx, p, []appdto.TagDefinition{{ID: late, Name: "late", Type: "integer", Version: 1}}); err != nil {
		t.Fatalf("UpsertTagDefinitions: %v", err)
	}

	policy := application.NewProjectCleanupPolicy(repositories.NewProjectRepositoryPostgres(testDB), newScopes(),
		stateUpsertingAfterRemoval{RuntimeNode: newRuntimeNode(), project: p, tagID: late})
	if err := policy.Run(ctx, "project:"+p.String()); !errors.Is(err, project.ErrProjectNotEmpty) {
		t.Fatalf("the run removing the states before the upsert: expected ErrProjectNotEmpty, got %v", err)
	}
	if n := countRows(t, "runtime.tags", p); n != 1 {
		t.Fatalf("states of the project after the failed run: %d, want the late one", n)
	}
	policy = application.NewProjectCleanupPolicy(repositories.NewProjectRepositoryPostgres(testDB), newScopes(), newRuntimeNode())
	if err := policy.Run(ctx, "project:"+p.String()); err != nil {
		t.Fatalf("the next run: %v", err)
	}
	if n := countRows(t, "runtime.tags", p); n != 0 {
		t.Errorf("states of the removed project: %d", n)
	}
}

// stateUpsertingAfterRemoval removes the Project's states, then upserts
// the definition of tagID again, as a late call does.
type stateUpsertingAfterRemoval struct {
	application.RuntimeNode
	project uuid.UUID
	tagID   uuid.UUID
}

func (n stateUpsertingAfterRemoval) RemoveProjectTagStates(ctx context.Context, projectID uuid.UUID) error {
	if err := n.RuntimeNode.RemoveProjectTagStates(ctx, projectID); err != nil {
		return err
	}
	return n.RuntimeNode.UpsertTagDefinitions(ctx, n.project, []appdto.TagDefinition{{ID: n.tagID, Name: "late", Type: "integer", Version: 1}})
}

// TestWidgetReconciliation_PinSwitch_ReconcilesOnlyItsProject: a release
// dropping a port changes only the Projects that pin it (ADR 0012); a
// Project pinning the older one keeps its bindings until it switches too.
func TestWidgetReconciliation_PinSwitch_ReconcilesOnlyItsProject(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	other := r.createProject(t, "Line 1")
	wt := r.createType(t, "value", "alarm")
	if _, _, err := r.pinSvc.PutLibraryPin(ctx, other, r.libraries[wt.ID], 1, 0); err != nil {
		t.Fatalf("pin in the other project: %v", err)
	}
	inMain := r.addWidget(t, r.createScene(t, "main scene").ID, 1, wt, "value", "alarm")
	sc, err := r.sceneSvc.CreateScene(ctx, other, appdto.SceneInput{Name: "other scene", Width: 100, Height: 100})
	if err != nil {
		t.Fatalf("CreateScene: %v", err)
	}
	var bindings []appdto.PortBinding
	for _, port := range []string{"value", "alarm"} {
		created, err := r.tagSvc.CreateTag(ctx, other, appdto.CreateTagInput{Name: port, Type: "integer"})
		if err != nil {
			t.Fatalf("CreateTag: %v", err)
		}
		bindings = append(bindings, appdto.PortBinding{PortName: port, TagID: created.ID})
	}
	inOther, err := r.sceneSvc.CreateWidget(ctx, other, sc.ID, sc.Version, appdto.WidgetInput{
		Name: "w", Width: 100, Height: 100, OriginX: 0.5, OriginY: 0.5, TypeID: wt.ID, PortBindings: bindings,
	})
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	r.settle(t)

	r.keepPorts(t, wt, "value")
	r.settle(t)

	if got := r.boundPorts(t, inMain.SceneID, inMain.ID); !slices.Equal(got, []string{"value"}) {
		t.Errorf("Main's widget: bound to %v, want [value]", got)
	}
	w, err := r.sceneSvc.FindWidgetByID(ctx, other, sc.ID, inOther.ID)
	if err != nil || len(w.PortBindings) != 2 {
		t.Errorf("the other project's widget: %+v, %v; want bound to [value alarm]", w.PortBindings, err)
	}

	pins, err := r.pinSvc.FindLibraryPins(ctx, other)
	if err != nil || len(pins) != 1 {
		t.Fatalf("pins of the other project: %v, %v", pins, err)
	}
	if _, _, err := r.pinSvc.PutLibraryPin(ctx, other, pins[0].LibraryID, 2, pins[0].Version); err != nil {
		t.Fatalf("switch the other project: %v", err)
	}
	r.settle(t)
	w, err = r.sceneSvc.FindWidgetByID(ctx, other, sc.ID, inOther.ID)
	if err != nil || len(w.PortBindings) != 1 || w.PortBindings[0].PortName != "value" {
		t.Errorf("the other project's widget after its switch: %+v, %v; want bound to [value]", w.PortBindings, err)
	}
}
