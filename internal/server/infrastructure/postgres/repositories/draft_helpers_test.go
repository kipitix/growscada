package repositories_test

import (
	"context"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// testDrafts is the repository of the Drafts over the test database.
func testDrafts() draft.Repository {
	return repositories.NewDraftRepositoryPostgres(testDB, repositories.NewLibraryReleaseRepositoryPostgres(testDB))
}

// draftVersionOf returns the Version of the Project's Draft.
func draftVersionOf(t *testing.T, projectID id.ID[project.Project]) version.Version[draft.Draft] {
	t.Helper()
	found, ok, err := testDrafts().Read(editlock.System(context.Background()), editlock.ProjectScope(projectID))
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	if !ok {
		return version.Committed[draft.Draft]()
	}
	return found.Version()
}

// mustUpdate runs one write of the Project's Draft at its current Version,
// as the server does.
func mustUpdate(t *testing.T, projectID id.ID[project.Project], description string, change func(*draft.Session) error) draft.Draft {
	t.Helper()
	ctx := editlock.System(context.Background())
	result, err := testDrafts().Update(ctx, editlock.ProjectScope(projectID), draftVersionOf(t, projectID), description, change)
	if err != nil {
		t.Fatalf("draft update: %v", err)
	}
	return result
}

// mustSave stores a Scene with its Widgets through the Draft and returns it as
// the repository reads it back.
func mustSave(t *testing.T, repo scene.SceneRepository, sc scene.Scene) scene.Scene {
	t.Helper()
	ctx := editlock.System(context.Background())
	mustUpdate(t, sc.ProjectID(), "test scene", func(session *draft.Session) error {
		session.CreateScene(ctx, sc)
		return nil
	})
	for _, w := range sc.Widgets() {
		widget := w
		mustUpdate(t, sc.ProjectID(), "test widget", func(session *draft.Session) error {
			_, err := session.AddWidget(ctx, sc.ID(), widget)
			return err
		})
	}
	found, err := repo.FindByID(ctx, sc.ID())
	if err != nil {
		t.Fatalf("read the scene back: %v", err)
	}
	return found
}
