package repositories_test

import (
	"context"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

func cleanScenes(t *testing.T) {
	t.Helper()
	// widgets cascade-delete with their scene (ON DELETE CASCADE on scene_id).
	if _, err := testDB.ExecContext(context.Background(), "DELETE FROM engineering.scenes"); err != nil {
		t.Fatalf("cleanScenes: %v", err)
	}
}

func makeScene(t *testing.T, repo scene.SceneRepository, name string) scene.Scene {
	t.Helper()
	sceneName, err := scene.NewSceneName(name)
	if err != nil {
		t.Fatalf("NewSceneName(%q): %v", name, err)
	}
	size, err := scene.NewSceneSize(1920, 1080)
	if err != nil {
		t.Fatalf("NewSceneSize: %v", err)
	}
	return scene.ReconstituteScene(repo.NextID(), mainProject, sceneName, size, scene.NewBackgroundHTML(""), nil)
}

func mustSaveScene(t *testing.T, repo scene.SceneRepository, name string) scene.Scene {
	t.Helper()
	return mustSave(t, repo, makeScene(t, repo, name))
}

// makeWidget builds a new Widget, of a WidgetType pinned in the project Main,
// for the scenes of repo.
func makeWidget(t *testing.T, repo scene.SceneRepository, name string) scene.Widget {
	t.Helper()
	return makeWidgetIn(t, repo, mainProject, name)
}

// makeWidgetIn builds a new Widget, of a WidgetType pinned in projectID, for
// the scenes of repo, a repository of that project.
func makeWidgetIn(t *testing.T, repo scene.SceneRepository, projectID id.ID[project.Project], name string) scene.Widget {
	t.Helper()
	newName, err := scene.NewWidgetName(name)
	if err != nil {
		t.Fatalf("NewWidgetName(%q): %v", name, err)
	}
	typeID := mustPinWidgetType(t, projectID).ID()
	return scene.NewWidget(
		repo.NextWidgetID(), newName, scene.NewPosition(10.0, 20.0, 0), library.DefaultSize(),
		scene.DefaultOrigin(), scene.DefaultRotation(),
		typeID, []string{"label1"}, nil,
	)
}

func withWidgets(sc scene.Scene, someWidgets ...scene.Widget) scene.Scene {
	return scene.ReconstituteScene(sc.ID(), sc.ProjectID(), sc.Name(), sc.Size(), sc.BackgroundHTML(), someWidgets)
}

func makeTag(t *testing.T, name string, repo tag.TagRepository) tag.Tag {
	t.Helper()
	tagName, err := tag.NewTagName(name)
	if err != nil {
		t.Fatalf("NewTagName(%q): %v", name, err)
	}
	newTag, err := tag.ReconstituteTag(repo.NextID(), mainProject, tagName, tag.TagTypeInteger, tag.NoSupplier())
	if err != nil {
		t.Fatalf("NewTag: %v", err)
	}
	return newTag
}

func mustSaveTag(t *testing.T, _ tag.TagRepository, aTag tag.Tag) tag.Tag {
	t.Helper()
	ctx := context.Background()
	mustUpdate(t, aTag.ProjectID(), "test tag", func(session *draft.Session) error {
		session.CreateTag(ctx, aTag)
		return nil
	})
	return aTag
}

func cleanTags(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		`DELETE FROM runtime.revisions; DELETE FROM runtime.tags; DELETE FROM engineering.tags`); err != nil {
		t.Fatalf("cleanTags: %v", err)
	}
}

// mustFindByID reads the Scene sceneID, failing the test on an error.
func mustFindByID(t *testing.T, repo scene.SceneRepository, sceneID id.ID[scene.Scene]) scene.Scene {
	t.Helper()
	found, err := repo.FindByID(context.Background(), sceneID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	return found
}
