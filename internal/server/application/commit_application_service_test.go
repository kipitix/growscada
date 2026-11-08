package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// newCommitService returns the service of Commits over the test database.
func newCommitService() application.CommitService {
	content := contentOver(newScopes())
	return application.NewCommitService(content, newLibraryContent(),
		application.NewProjectFileBuilder(content), repositories.NewWidgetTypeRepositories(testDB),
		repositories.NewProjectReleaseRepositoryPostgres(testDB))
}

// TestCommitService_ProjectSaveListFindRename: a Save of a Project's Draft
// numbers its Commits, a Save without changes is refused, and the history,
// the ProjectFile and the title rename work (task 72_2; ADR 0021).
func TestCommitService_ProjectSaveListFindRename(t *testing.T) {
	cleanProjects(t)
	ctx := context.Background()
	projects := application.NewProjectService(repositories.NewProjectRepositoryPostgres(testDB), soleOrganization())
	createdProject, err := projects.CreateProject(ctx, appdto.ProjectInput{Name: "commits-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	service := newCommitService()
	scope := draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](createdProject.ID)))

	saved, err := service.Save(ctx, scope, 1, "First save", "Ada")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Number != 1 || saved.Title != "First save" || saved.Author != "Ada" || saved.Released {
		t.Errorf("saved commit: %+v", saved)
	}
	if saved.DraftVersion != 1 {
		t.Errorf("the commit fixed version %d, want 1", saved.DraftVersion)
	}

	if _, err := service.Save(ctx, scope, 1, "Again", "Ada"); !errors.Is(err, draft.ErrNothingToSave) {
		t.Fatalf("Save without changes: %v, want ErrNothingToSave", err)
	}
	if _, err := service.Save(ctx, scope, 0, "", "Ada"); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("Save with an empty title: %v, want invalid input", err)
	}
	if _, err := service.Save(ctx, scope, 0, "Title", ""); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("Save with an empty author: %v, want invalid input", err)
	}

	list, err := service.List(ctx, scope)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Number != 1 {
		t.Errorf("history: %+v", list)
	}

	found, err := service.Find(ctx, scope, 1)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.Title != "First save" || found.Counts.Tags != 0 {
		t.Errorf("found commit: %+v", found)
	}
	if _, err := service.Find(ctx, scope, 9); !errors.Is(err, draft.ErrCommitNotFound) {
		t.Errorf("Find of a missing commit: %v, want ErrCommitNotFound", err)
	}

	renamed, err := service.Rename(ctx, scope, 1, "  Before the demo ")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.Title != "Before the demo" {
		t.Errorf("renamed title %q", renamed.Title)
	}
	if _, err := service.Rename(ctx, scope, 1, " "); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("Rename with an empty title: %v, want invalid input", err)
	}

	download, err := service.ProjectFile(ctx, scope, 1)
	if err != nil {
		t.Fatalf("ProjectFile: %v", err)
	}
	if download.File.ProjectID != createdProject.ID || download.ProjectName != createdProject.Name {
		t.Errorf("download of project %s / %q", download.File.ProjectID, download.ProjectName)
	}
}

// TestCommitService_LibrarySaveAndList: a Library's Draft is saved the same
// way, its Commit holds its WidgetTypes, and a Project's download is refused
// for a Library's scope (task 72_2; ADR 0021).
func TestCommitService_LibrarySaveAndList(t *testing.T) {
	ctx := context.Background()
	libraries := application.NewLibraryService(newLibraryContent(), repositories.NewLibraryRepositoryPostgres(testDB),
		repositories.NewLibraryReleaseRepositoryPostgres(testDB), repositories.NewWidgetTypeRepositories(testDB))
	created, err := libraries.CreateLibrary(ctx, appdto.LibraryInput{Name: "commits-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	service := newCommitService()
	scope := draft.LibraryScope(id.NewID(id.IDWithUUID[library.Library](created.ID)))

	saved, err := service.Save(ctx, scope, 1, "Widgets", "Ada")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Number != 1 {
		t.Errorf("saved commit number %d", saved.Number)
	}
	found, err := service.Find(ctx, scope, 1)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.WidgetTypes == nil {
		t.Errorf("a library's commit carries no widget types list")
	}
	if _, err := service.ProjectFile(ctx, scope, 1); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("ProjectFile of a library's commit: %v, want invalid input", err)
	}
}
