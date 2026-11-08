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
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/release"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// newReleaseService returns the service of Project Releases over the test
// database.
func newReleaseService() application.ReleaseService {
	return application.NewReleaseService(contentOver(newScopes()), repositories.NewProjectReleaseRepositoryPostgres(testDB),
		newRuntimeNode(), repositories.NewLibraryRepositoryPostgres(testDB),
		repositories.NewLibraryReleaseRepositoryPostgres(testDB))
}

// TestReleaseService_CreateListFindNoteAndFile: a Commit is published once,
// the Releases are listed and read, a note changes and the copied ProjectFile
// downloads (task 72_3; ADR 0021).
func TestReleaseService_CreateListFindNoteAndFile(t *testing.T) {
	cleanProjects(t)
	ctx := context.Background()
	projects := application.NewProjectService(repositories.NewProjectRepositoryPostgres(testDB), soleOrganization())
	createdProject, err := projects.CreateProject(ctx, appdto.ProjectInput{Name: "releases-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := createdProject.ID
	commits := newCommitService()
	scope := draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID)))
	if _, err := commits.Save(ctx, scope, 1, "First save", "Ada"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	service := newReleaseService()

	created, err := service.Create(ctx, projectID, 1, " for the demo ", "Grace")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Number != 1 || created.CommitNumber != 1 || created.Title != "First save" ||
		created.Note != "for the demo" || created.Author != "Grace" || created.Origin != "commit" {
		t.Errorf("created release: %+v", created)
	}
	if _, err := service.Create(ctx, projectID, 1, "", "Grace"); !errors.Is(err, release.ErrCommitAlreadyReleased) {
		t.Errorf("a second release of a commit: %v, want ErrCommitAlreadyReleased", err)
	}
	if _, err := service.Create(ctx, projectID, 9, "", "Grace"); !errors.Is(err, draft.ErrCommitNotFound) {
		t.Errorf("a release of a missing commit: %v, want ErrCommitNotFound", err)
	}
	if _, err := service.Create(ctx, projectID, 1, "", ""); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("a release without an author: %v, want invalid input", err)
	}
	if _, err := service.Create(ctx, projectID, 0, "", "Grace"); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("a release of commit 0: %v, want invalid input", err)
	}

	// The Commit list marks the published one.
	list, err := commits.List(ctx, scope)
	if err != nil {
		t.Fatalf("List commits: %v", err)
	}
	if len(list) != 1 || !list[0].Released {
		t.Errorf("the published commit is not marked: %+v", list)
	}
	// Renaming a published Commit is metadata: it keeps its Release mark.
	renamed, err := commits.Rename(ctx, scope, 1, "Renamed after the release")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if !renamed.Released {
		t.Errorf("a renamed published commit lost its release mark: %+v", renamed)
	}

	releases, err := service.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(releases) != 1 || releases[0].Number != 1 {
		t.Errorf("releases: %+v", releases)
	}
	found, err := service.Find(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.Title != "First save" || found.Counts.Scenes != 0 {
		t.Errorf("found release: %+v", found)
	}
	if _, err := service.Find(ctx, projectID, 9); !errors.Is(err, release.ErrReleaseNotFound) {
		t.Errorf("Find of a missing release: %v, want ErrReleaseNotFound", err)
	}

	updated, err := service.UpdateNote(ctx, projectID, 1, "after the demo")
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}
	if updated.Note != "after the demo" {
		t.Errorf("the note is %q", updated.Note)
	}

	download, err := service.ProjectFile(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("ProjectFile: %v", err)
	}
	if download.File.ProjectID != projectID || download.ProjectName != createdProject.Name {
		t.Errorf("download of project %s / %q", download.File.ProjectID, download.ProjectName)
	}
}

// TestReleaseService_ListNeedsNoRuntime: a Release list is the Engineering
// node's data, so it is served while the Runtime node is unreachable (task
// 72_4); the running Release is the Deployment's to name, by another read.
func TestReleaseService_ListNeedsNoRuntime(t *testing.T) {
	cleanProjects(t)
	ctx := context.Background()
	projects := application.NewProjectService(repositories.NewProjectRepositoryPostgres(testDB), soleOrganization())
	createdProject, err := projects.CreateProject(ctx, appdto.ProjectInput{Name: "releases-no-runtime-" + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := createdProject.ID
	scope := draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID)))
	if _, err := newCommitService().Save(ctx, scope, 1, "First save", "Ada"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := newReleaseService().Create(ctx, projectID, 1, "", "Ada"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	service := application.NewReleaseService(contentOver(newScopes()),
		repositories.NewProjectReleaseRepositoryPostgres(testDB), nil,
		repositories.NewLibraryRepositoryPostgres(testDB), repositories.NewLibraryReleaseRepositoryPostgres(testDB))
	list, err := service.List(ctx, projectID)
	if err != nil || len(list) != 1 || list[0].Number != 1 {
		t.Errorf("List without a Runtime node: %+v, %v", list, err)
	}
}

// TestReleaseService_RefusesADeletedProject: a Project that is not live has no
// Releases to read or publish.
func TestReleaseService_RefusesADeletedProject(t *testing.T) {
	service := newReleaseService()
	if _, err := service.List(context.Background(), uuid.New()); !errors.Is(err, project.ErrProjectNotFound) {
		t.Errorf("List of a missing project: %v, want ErrProjectNotFound", err)
	}
}
