package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// cleanProjects removes every project but mainProject, with the content of
// all of them.
func cleanProjects(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		"DELETE FROM runtime.tags; DELETE FROM engineering.tags; DELETE FROM engineering.scenes; DELETE FROM engineering.library_pins; "+
			"DELETE FROM projects WHERE id <> '"+mainProject.String()+"'"); err != nil {
		t.Fatalf("cleanProjects: %v", err)
	}
}

func soleOrganization(t *testing.T) id.ID[organization.Organization] {
	t.Helper()
	orgID, err := repositories.FindSoleOrganization(context.Background(), testDB)
	if err != nil {
		t.Fatalf("FindSoleOrganization: %v", err)
	}
	return orgID
}

func mustProjectName(t *testing.T, s string) project.ProjectName {
	t.Helper()
	name, err := project.NewProjectName(s)
	if err != nil {
		t.Fatalf("NewProjectName(%q): %v", s, err)
	}
	return name
}

// mustCreateProject stores a new project of the sole organization.
func mustCreateProject(t *testing.T, repo project.ProjectRepository, name string) project.Project {
	t.Helper()
	saved, err := repo.Save(context.Background(), project.CreateProject(repo.NextID(), soleOrganization(t), mustProjectName(t, name)))
	if err != nil {
		t.Fatalf("Save project %q: %v", name, err)
	}
	return saved
}

func TestFindSoleOrganization_IsTheOneTheMigrationsCreate(t *testing.T) {
	if got := soleOrganization(t).String(); got != "a0000000-0000-4000-8000-000000000001" {
		t.Errorf("organization %s", got)
	}
	main, err := repositories.NewProjectRepositoryPostgres(testDB).FindByID(context.Background(), mainProject)
	if err != nil {
		t.Fatalf("the migrations' project Main: %v", err)
	}
	if main.Name().String() != "Main" || main.OrganizationID() != soleOrganization(t) {
		t.Errorf("unexpected project %s", main)
	}
}

func TestProjectSave_NewProject_IsFoundByIDAndName(t *testing.T) {
	cleanProjects(t)
	repo := repositories.NewProjectRepositoryPostgres(testDB)
	ctx := context.Background()

	saved := mustCreateProject(t, repo, "Line 1")

	if saved.Version().Number() != 1 || len(saved.PendingEvents()) != 0 {
		t.Errorf("unexpected saved project %s", saved)
	}
	byID, err := repo.FindByID(ctx, saved.ID())
	if err != nil || byID.Name().String() != "Line 1" {
		t.Errorf("FindByID: %v, %v", byID, err)
	}
	byName, err := repo.FindByName(ctx, soleOrganization(t), mustProjectName(t, "Line 1"))
	if err != nil || byName.ID() != saved.ID() {
		t.Errorf("FindByName: %v, %v", byName, err)
	}
}

func TestProjectSave_TakenName_ReturnsErrProjectNameTaken(t *testing.T) {
	cleanProjects(t)
	repo := repositories.NewProjectRepositoryPostgres(testDB)
	mustCreateProject(t, repo, "Line 1")

	_, err := repo.Save(context.Background(), project.CreateProject(repo.NextID(), soleOrganization(t), mustProjectName(t, "Line 1")))

	if !errors.Is(err, project.ErrProjectNameTaken) {
		t.Errorf("expected ErrProjectNameTaken, got %v", err)
	}
}

func TestProjectSave_Rename_RaisesVersionAndStaleOneConflicts(t *testing.T) {
	cleanProjects(t)
	repo := repositories.NewProjectRepositoryPostgres(testDB)
	ctx := context.Background()
	saved := mustCreateProject(t, repo, "Line 1")

	renamed, err := saved.Rename(saved.Version(), mustProjectName(t, "Line 2"))
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	stored, err := repo.Save(ctx, renamed)
	if err != nil || stored.Version().Number() != 2 || stored.Name().String() != "Line 2" {
		t.Fatalf("Save renamed: %v, %v", stored, err)
	}

	if _, err := repo.Save(ctx, renamed); !errors.Is(err, project.ErrProjectConflict) {
		t.Errorf("a rename at the old version: expected ErrProjectConflict, got %v", err)
	}
}

func TestProjectSave_Deleted_IsGoneForReadsAndFreesTheName(t *testing.T) {
	cleanProjects(t)
	repo := repositories.NewProjectRepositoryPostgres(testDB)
	ctx := context.Background()
	saved := mustCreateProject(t, repo, "Line 1")

	if _, err := repo.Save(ctx, saved.Delete()); err != nil {
		t.Fatalf("Save deleted: %v", err)
	}

	if _, err := repo.FindByID(ctx, saved.ID()); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("FindByID of a deleted project: expected ErrProjectNotFound, got %v", err)
	}
	all, err := repo.FindAll(ctx, soleOrganization(t))
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	for _, p := range all {
		if p.ID() == saved.ID() {
			t.Error("FindAll lists a deleted project")
		}
	}
	deleted, err := repo.FindDeleted(ctx, saved.ID())
	if err != nil || !deleted.IsDeleted() {
		t.Errorf("FindDeleted: %v, %v", deleted, err)
	}
	if _, err := repo.FindDeleted(ctx, mainProject); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("FindDeleted of a live project: expected ErrProjectNotFound, got %v", err)
	}
	mustCreateProject(t, repo, "Line 1") // the name is free
	if _, err := repo.Save(ctx, saved.Delete()); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("deleting a deleted project again: expected ErrProjectNotFound, got %v", err)
	}
}

func TestProjectFindAll_LiveProjectsInCreationOrder(t *testing.T) {
	cleanProjects(t)
	repo := repositories.NewProjectRepositoryPostgres(testDB)
	b := mustCreateProject(t, repo, "B")
	a := mustCreateProject(t, repo, "A")

	all, err := repo.FindAll(context.Background(), soleOrganization(t))

	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	var got []id.ID[project.Project]
	for _, p := range all {
		got = append(got, p.ID())
	}
	want := []id.ID[project.Project]{mainProject, b.ID(), a.ID()}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("FindAll: got %v, want %v", got, want)
	}
	other, err := repo.FindAll(context.Background(), id.NewID[organization.Organization]())
	if err != nil || len(other) != 0 {
		t.Errorf("FindAll of another organization: %v, %v", other, err)
	}
}

func TestProjectDelete_RefusedWhileContentRemains(t *testing.T) {
	cleanProjects(t)
	repo := repositories.NewProjectRepositoryPostgres(testDB)
	ctx := context.Background()
	p := mustCreateProject(t, repo, "Line 1")
	tags := repositories.NewTagRepositoryPostgres(testDB, p.ID())
	created := createTagIn(t, tags, p.ID(), "speed")
	deleted, err := repo.Save(ctx, p.Delete())
	if err != nil {
		t.Fatalf("Save deleted: %v", err)
	}

	if err := repo.Delete(ctx, deleted); !errors.Is(err, project.ErrProjectNotEmpty) {
		t.Fatalf("Delete with a tag left: expected ErrProjectNotEmpty, got %v", err)
	}
	if err := tags.Delete(ctx, created.Delete()); err != nil {
		t.Fatalf("delete tag: %v", err)
	}
	if err := repo.Delete(ctx, deleted); err != nil {
		t.Fatalf("Delete of an empty project: %v", err)
	}
	if _, err := repo.FindDeleted(ctx, p.ID()); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("a removed project: expected ErrProjectNotFound, got %v", err)
	}
	if err := repo.Delete(ctx, deleted); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("Delete again: expected ErrProjectNotFound, got %v", err)
	}
}

func TestProjectDelete_LiveProject_IsNotRemoved(t *testing.T) {
	cleanProjects(t)
	repo := repositories.NewProjectRepositoryPostgres(testDB)
	p := mustCreateProject(t, repo, "Line 1")

	if err := repo.Delete(context.Background(), p); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("Delete of a live project: expected ErrProjectNotFound, got %v", err)
	}
}

// ══════════════════════ Isolation of the projects' content ══════════════════

// createTagIn stores a new Integer tag of the project through repo.
func createTagIn(t *testing.T, repo tag.TagRepository, projectID id.ID[project.Project], name string) tag.Tag {
	t.Helper()
	tagName, _ := tag.NewTagName(name)
	created, err := tag.CreateTag(repo.NextID(), projectID, tagName, tag.TagTypeInteger)
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	saved, err := repo.Save(context.Background(), created)
	if err != nil {
		t.Fatalf("Save tag %q: %v", name, err)
	}
	return saved
}

func TestTagRepository_TagsOfProjectsAreApart(t *testing.T) {
	cleanProjects(t)
	other := mustCreateProject(t, repositories.NewProjectRepositoryPostgres(testDB), "Line 1")
	mainTags := repositories.NewTagRepositoryPostgres(testDB, mainProject)
	otherTags := repositories.NewTagRepositoryPostgres(testDB, other.ID())
	ctx := context.Background()

	inMain := createTagIn(t, mainTags, mainProject, "speed")
	inOther := createTagIn(t, otherTags, other.ID(), "speed") // the same name in another project

	if found, err := mainTags.FindByName(ctx, inMain.Name()); err != nil || found.ID() != inMain.ID() || found.ProjectID() != mainProject {
		t.Errorf("FindByName in Main: %v, %v", found, err)
	}
	if _, err := mainTags.FindByID(ctx, inOther.ID()); !errors.Is(err, tag.ErrTagNotFound) {
		t.Errorf("FindByID of another project's tag: expected ErrTagNotFound, got %v", err)
	}
	types, err := mainTags.FindTypesByIDs(ctx, []id.ID[tag.Tag]{inMain.ID(), inOther.ID()})
	if _, foreign := types[inOther.ID()]; err != nil || len(types) != 1 || foreign {
		t.Errorf("FindTypesByIDs: %v, %v; want only Main's tag", types, err)
	}
	all, err := otherTags.FindAll(ctx)
	if err != nil || len(all) != 1 || all[0].ID() != inOther.ID() || all[0].ProjectID() != other.ID() {
		t.Errorf("FindAll of the other project: %v, %v", all, err)
	}
	if _, err := mainTags.Save(ctx, inOther); err == nil {
		t.Error("Save of another project's tag: expected an error")
	}
	if err := mainTags.Delete(ctx, inOther.Delete()); err == nil {
		t.Error("Delete of another project's tag: expected an error")
	}
	if _, err := otherTags.FindByID(ctx, inOther.ID()); err != nil {
		t.Errorf("the other project's tag must survive: %v", err)
	}
}

func TestTagRepository_SaveInARemovedProject_IsProjectNotFound(t *testing.T) {
	cleanProjects(t)
	gone := id.NewID[project.Project]() // no such row: as if its cleanup had run
	repo := repositories.NewTagRepositoryPostgres(testDB, gone)
	tagName, _ := tag.NewTagName("speed")
	created, _ := tag.CreateTag(repo.NextID(), gone, tagName, tag.TagTypeInteger)

	if _, err := repo.Save(context.Background(), created); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB, gone)
	name, _ := scene.NewSceneName("s")
	size, _ := scene.NewSceneSize(10, 10)
	if _, err := sceneRepo.Save(context.Background(), scene.CreateScene(sceneRepo.NextID(), gone, name, size, scene.NewBackgroundHTML(""))); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("scene: expected ErrProjectNotFound, got %v", err)
	}
}

func TestSceneRepository_ScenesOfProjectsAreApart(t *testing.T) {
	cleanProjects(t)
	other := mustCreateProject(t, repositories.NewProjectRepositoryPostgres(testDB), "Line 1")
	mainScenes := repositories.NewSceneRepositoryPostgres(testDB, mainProject)
	otherScenes := repositories.NewSceneRepositoryPostgres(testDB, other.ID())
	ctx := context.Background()

	inMain := mustSaveScene(t, mainScenes, "overview")
	name, _ := scene.NewSceneName("overview")
	size, _ := scene.NewSceneSize(100, 100)
	w := makeWidgetIn(t, otherScenes, other.ID(), "w1")
	inOther := mustSave(t, otherScenes, withWidgets(
		scene.CreateScene(otherScenes.NextID(), other.ID(), name, size, scene.NewBackgroundHTML("")), w))

	if _, err := mainScenes.FindByID(ctx, inOther.ID()); !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("FindByID of another project's scene: expected ErrSceneNotFound, got %v", err)
	}
	all, err := mainScenes.FindAll(ctx)
	if err != nil || len(all) != 1 || all[0].ID() != inMain.ID() || all[0].ProjectID() != mainProject {
		t.Errorf("FindAll of Main: %v, %v", all, err)
	}
	found := mustFindByID(t, otherScenes, inOther.ID())
	if found.ProjectID() != other.ID() || len(found.Widgets()) != 1 {
		t.Errorf("the other project's scene: %s with %d widgets", found, len(found.Widgets()))
	}
	if _, err := mainScenes.Save(ctx, found); err == nil {
		t.Error("Save of another project's scene: expected an error")
	}
	if err := mainScenes.Delete(ctx, found.Delete()); err == nil {
		t.Error("Delete of another project's scene: expected an error")
	}

	var widgetProject uuid.UUID
	if err := testDB.QueryRowContext(ctx, `SELECT project_id FROM engineering.widgets WHERE id = $1`, w.ID().UUID()).Scan(&widgetProject); err != nil {
		t.Fatalf("read widget row: %v", err)
	}
	if widgetProject != other.ID().UUID() {
		t.Errorf("widget row of project %s, want its scene's %s", widgetProject, other.ID())
	}
}

// TestProjectDelete_PinsHoldTheRow: a project's row is removed only once its
// Library Pins are gone, as its other content.
func TestProjectDelete_PinsHoldTheRow(t *testing.T) {
	cleanProjects(t)
	projects := repositories.NewProjectRepositoryPostgres(testDB)
	other := mustCreateProject(t, projects, "Line 1")
	ctx := context.Background()
	mustPinWidgetType(t, other.ID())
	if _, err := projects.Save(ctx, other.Delete()); err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	deleted, err := projects.FindDeleted(ctx, other.ID())
	if err != nil {
		t.Fatalf("FindDeleted: %v", err)
	}
	if err := projects.Delete(ctx, deleted); !errors.Is(err, project.ErrProjectNotEmpty) {
		t.Errorf("with a pin: expected ErrProjectNotEmpty, got %v", err)
	}
	pins := repositories.NewLibraryPinRepositoryPostgres(testDB, other.ID())
	all, err := pins.FindAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("pins: %v, %v", all, err)
	}
	if err := pins.Delete(ctx, all[0].Delete(), nil); err != nil {
		t.Fatalf("delete pin: %v", err)
	}
	if err := projects.Delete(ctx, deleted); err != nil {
		t.Errorf("without pins: %v", err)
	}
}

// TestInLiveProjectLists_CheckTheProject: the lists the UI reloads on every
// event read the content of a live Project of the Organization only.
func TestInLiveProjectLists_CheckTheProject(t *testing.T) {
	cleanProjects(t)
	projects := repositories.NewProjectRepositoryPostgres(testDB)
	line := mustCreateProject(t, projects, "Line 1")
	scopes := repositories.NewProjectScopes(testDB)
	tags, scenes := scopes.ForProject(line.ID()).Tags(), scopes.ForProject(line.ID()).Scenes()
	ctx := context.Background()
	createTagIn(t, tags, line.ID(), "speed")
	name, _ := scene.NewSceneName("s")
	size, _ := scene.NewSceneSize(100, 100)
	sc := mustSave(t, scenes, withWidgets(scene.CreateScene(scenes.NextID(), line.ID(), name, size, scene.NewBackgroundHTML("")), makeWidgetIn(t, scenes, line.ID(), "w")))
	org, other := soleOrganization(t), id.NewID[organization.Organization]()
	check := func(when string, orgID id.ID[organization.Organization], want int) {
		t.Helper()
		if got, err := tags.FindAllInLiveProject(ctx, orgID); err != nil || len(got) != want {
			t.Errorf("%s: tags %v, %v; want %d", when, got, err, want)
		}
		if got, err := scenes.FindAllInLiveProject(ctx, orgID); err != nil || len(got) != want {
			t.Errorf("%s: scenes %v, %v; want %d", when, got, err, want)
		}
		found, err := scenes.FindByIDInLiveProject(ctx, sc.ID(), orgID)
		switch {
		case want == 1 && (err != nil || len(found.Widgets()) != 1):
			t.Errorf("%s: scene %v, %v; want it with its widget", when, found, err)
		case want == 0 && !errors.Is(err, scene.ErrSceneNotFound):
			t.Errorf("%s: scene: expected ErrSceneNotFound, got %v", when, err)
		}
	}

	check("live", org, 1)
	check("through another organization", other, 0)
	if _, err := projects.Save(ctx, line.Delete()); err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	check("marked deleted", org, 0)
	if all, err := tags.FindAll(ctx); err != nil || len(all) != 1 {
		t.Errorf("FindAll still reads the tags, for the cleanup: %v, %v", all, err)
	}
}
