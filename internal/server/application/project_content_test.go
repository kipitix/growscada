package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// countingProjects counts the reads of a Project by ID.
type countingProjects struct {
	project.ProjectRepository
	reads int
}

func (c *countingProjects) FindByID(ctx context.Context, projectID id.ID[project.Project]) (project.Project, error) {
	c.reads++
	return c.ProjectRepository.FindByID(ctx, projectID)
}

// TestHotLists_ReadTheProjectOnlyToTellWhyNothingIsFound: the lists the UI
// reloads on every event (Tags, Scenes, the Widgets of a Scene) check their
// Project in their own query; the Project is read apart only when nothing
// is found, to tell a missing or deleted Project from an empty one.
func TestHotLists_ReadTheProjectOnlyToTellWhyNothingIsFound(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	p := r.createProject(t, "Line 1")
	projects := &countingProjects{ProjectRepository: repositories.NewProjectRepositoryPostgres(testDB)}
	content := application.NewProjectContent(projects, newScopes(), soleOrganization())
	tags := application.NewTagService(content)
	scenes := application.NewSceneService(content, repositories.NewWidgetTypeRepositoryPostgres(testDB))
	reads := func(when string, want int) {
		t.Helper()
		if projects.reads != want {
			t.Errorf("%s: the project read %d times, want %d", when, projects.reads, want)
		}
		projects.reads = 0
	}

	if list, err := tags.FindAllTags(ctx, p); err != nil || len(list) != 0 {
		t.Errorf("tags of an empty project: %v, %v", list, err)
	}
	if list, err := scenes.FindAllScenes(ctx, p); err != nil || len(list) != 0 {
		t.Errorf("scenes of an empty project: %v, %v", list, err)
	}
	if _, err := scenes.FindWidgetsBySceneID(ctx, p, uuid.New()); !errors.Is(err, scene.ErrSceneNotFound) {
		t.Errorf("widgets of a missing scene: expected ErrSceneNotFound, got %v", err)
	}
	reads("nothing found", 3)

	if _, err := r.tagSvc.CreateTag(ctx, p, appdto.CreateTagInput{Name: "speed", Type: "integer", Value: "0", Quality: "good"}); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	sc, err := r.sceneSvc.CreateScene(ctx, p, appdto.SceneInput{Name: "s", Width: 100, Height: 100})
	if err != nil {
		t.Fatalf("CreateScene: %v", err)
	}
	if list, err := tags.FindAllTags(ctx, p); err != nil || len(list) != 1 {
		t.Errorf("tags: %v, %v", list, err)
	}
	if list, err := scenes.FindAllScenes(ctx, p); err != nil || len(list) != 1 {
		t.Errorf("scenes: %v, %v", list, err)
	}
	if list, err := scenes.FindWidgetsBySceneID(ctx, p, sc.ID); err != nil || len(list) != 0 {
		t.Errorf("widgets of the scene: %v, %v", list, err)
	}
	reads("found", 0)

	if _, err := r.projectSvc.DeleteProjectByID(ctx, p); err != nil {
		t.Fatalf("DeleteProjectByID: %v", err)
	}
	if _, err := tags.FindAllTags(ctx, p); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("tags of a deleted project: expected ErrProjectNotFound, got %v", err)
	}
	if _, err := scenes.FindAllScenes(ctx, p); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("scenes of a deleted project: expected ErrProjectNotFound, got %v", err)
	}
	if _, err := scenes.FindWidgetsBySceneID(ctx, p, sc.ID); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("widgets of a deleted project's scene: expected ErrProjectNotFound, got %v", err)
	}
}
