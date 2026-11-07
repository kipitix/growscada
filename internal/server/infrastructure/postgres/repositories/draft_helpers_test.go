package repositories_test

import (
	"context"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
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
	found, ok, err := testDrafts().Read(context.Background(), draft.ProjectScope(projectID))
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	if !ok {
		return version.Committed[draft.Draft]()
	}
	return found.Version()
}

// libraryDraftVersionOf returns the Version of the Library's Draft.
func libraryDraftVersionOf(t *testing.T, libraryID id.ID[library.Library]) version.Version[draft.Draft] {
	t.Helper()
	found, ok, err := testDrafts().Read(context.Background(), draft.LibraryScope(libraryID))
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	if !ok {
		return version.Committed[draft.Draft]()
	}
	return found.Version()
}

// libraryDraftContentVersionOf is libraryDraftVersionOf as the Library's
// release repository states it.
func libraryDraftContentVersionOf(t *testing.T, libraryID id.ID[library.Library]) version.Version[library.DraftContent] {
	t.Helper()
	ver, err := version.New(version.WithNumber[library.DraftContent](libraryDraftVersionOf(t, libraryID).Number()))
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	return ver
}

// mustUpdateLibrary runs one write of the Library's Draft at its current
// Version, as the server does.
func mustUpdateLibrary(t *testing.T, libraryID id.ID[library.Library], description string, change func(*draft.Session) error) draft.Draft {
	t.Helper()
	ctx := context.Background()
	result, err := testDrafts().Update(ctx, draft.LibraryScope(libraryID), libraryDraftVersionOf(t, libraryID), description, change)
	if err != nil {
		t.Fatalf("library draft update: %v", err)
	}
	return result
}

// mustCreateWidgetType stores wt in the Library's Draft and returns it as the
// read repository sees it.
func mustCreateWidgetType(t *testing.T, libraryID id.ID[library.Library], wt library.WidgetType) library.WidgetType {
	t.Helper()
	ctx := context.Background()
	mustUpdateLibrary(t, libraryID, "Create widget type "+wt.Name().String(), func(session *draft.Session) error {
		session.CreateWidgetType(ctx, wt)
		return nil
	})
	found, err := repositories.NewWidgetTypeRepositoryPostgres(testDB, libraryID).FindByID(ctx, wt.ID())
	if err != nil {
		t.Fatalf("read widget type: %v", err)
	}
	return found
}

// mustUpdateLibraryWidgetType stores wt as the new state of the WidgetType of
// the Library's Draft.
func mustUpdateLibraryWidgetType(t *testing.T, libraryID id.ID[library.Library], wt library.WidgetType) library.WidgetType {
	t.Helper()
	ctx := context.Background()
	mustUpdateLibrary(t, libraryID, "Update widget type "+wt.Name().String(), func(session *draft.Session) error {
		updated, err := session.UpdateWidgetType(ctx, wt.ID(), wt.Name(), wt.HtmlTemplate(), wt.Script(),
			wt.ScriptLanguage(), wt.DefaultSize(), wt.InputPorts())
		if err == nil {
			wt = updated
		}
		return err
	})
	found, err := repositories.NewWidgetTypeRepositoryPostgres(testDB, libraryID).FindByID(ctx, wt.ID())
	if err != nil {
		t.Fatalf("read widget type: %v", err)
	}
	return found
}

// mustUpdate runs one write of the Project's Draft at its current Version,
// as the server does.
func mustUpdate(t *testing.T, projectID id.ID[project.Project], description string, change func(*draft.Session) error) draft.Draft {
	t.Helper()
	ctx := context.Background()
	result, err := testDrafts().Update(ctx, draft.ProjectScope(projectID), draftVersionOf(t, projectID), description, change)
	if err != nil {
		t.Fatalf("draft update: %v", err)
	}
	return result
}

// mustSave stores a Scene with its Widgets through the Draft and returns it as
// the repository reads it back.
func mustSave(t *testing.T, repo scene.SceneRepository, sc scene.Scene) scene.Scene {
	t.Helper()
	ctx := context.Background()
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
