package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/description"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/libraryimport"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// mustSaveLibrary stores a new live library of a name of its own.
func mustSaveLibrary(t *testing.T) library.Library {
	t.Helper()
	name, err := library.NewLibraryName("lib-" + uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	repo := repositories.NewLibraryRepositoryPostgres(testDB)
	saved, err := repo.Save(context.Background(), library.CreateLibrary(repo.NextID(), soleOrganization(t), name, description.Description{}))
	if err != nil {
		t.Fatalf("Save library: %v", err)
	}
	return saved
}

// cleanWidgetTypes deletes the libraries, with their Drafts and saved states.
func cleanWidgetTypes(t *testing.T) {
	t.Helper()
	cleanLibraries(t)
}

// libraryRepo is the WidgetTypeRepository of a library, with the library's ID.
type libraryRepo struct {
	library.WidgetTypeRepository
	libraryID id.ID[library.Library]
}

// newWidgetTypeRepo returns the WidgetTypeRepository of a new library.
func newWidgetTypeRepo(t *testing.T) libraryRepo {
	t.Helper()
	lib := mustSaveLibrary(t)
	return libraryRepo{repositories.NewWidgetTypeRepositoryPostgres(testDB, lib.ID()), lib.ID()}
}

// mustCommit saves the Draft of the library as a Commit, or returns its last
// one when the Draft has not changed since it.
func mustCommit(t *testing.T, libraryID id.ID[library.Library]) (draft.Commit, error) {
	t.Helper()
	ctx := context.Background()
	draftNow, err := repositories.NewWidgetTypeRepositoryPostgres(testDB, libraryID).FindAll(ctx)
	if err != nil {
		return nil, err
	}
	copies := make([]library.WidgetTypeCopy, len(draftNow))
	for i, wt := range draftNow {
		copies[i] = library.CopyOf(wt)
	}
	title, err := draft.NewCommitTitle("Release")
	if err != nil {
		return nil, err
	}
	author, err := draft.NewAuthor("Ada")
	if err != nil {
		return nil, err
	}
	saved, err := testDrafts().SaveCommit(ctx, draft.LibraryScope(libraryID), libraryDraftVersionOf(t, libraryID),
		title, author, draft.NewLibraryCommitContent(copies))
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, draft.ErrNothingToSave) {
		return nil, err
	}
	last, ok, err := testDrafts().FindLastCommit(ctx, draft.LibraryScope(libraryID))
	if err != nil || !ok {
		return nil, err
	}
	return last, nil
}

// mustImport imports the saved state of the Library in the project through the
// project's Draft (task 85_6).
func mustImport(t *testing.T, projectID id.ID[project.Project], commit draft.Commit) libraryimport.LibraryImport {
	t.Helper()
	ctx := context.Background()
	var saved libraryimport.LibraryImport
	mustUpdate(t, projectID, "test import", func(session *draft.Session) error {
		var err error
		saved, _, err = session.PutLibraryImport(ctx, commit.Scope().LibraryID(), commit.Number())
		return err
	})
	return saved
}

// copyOfCommit returns the copy of the WidgetType typeID the Commit holds.
func copyOfCommit(t *testing.T, commit draft.Commit, typeID id.ID[library.WidgetType]) library.WidgetTypeCopy {
	t.Helper()
	copies, ok := commit.Content().WidgetTypes()
	if !ok {
		t.Fatalf("the commit holds no widget types")
	}
	for _, copy := range copies {
		if copy.ID() == typeID {
			return copy
		}
	}
	t.Fatalf("the commit lacks the widget type %s", typeID)
	return library.WidgetTypeCopy{}
}

// mustSaveWidgetType stores a new WidgetType with the given ports in a new
// library, saves it and imports it in the project Main: a Widget's type must be
// imported.
func mustSaveWidgetType(t *testing.T, ports ...library.InputPort) id.ID[library.WidgetType] {
	t.Helper()
	return mustImportWidgetType(t, mainProject, ports...).ID()
}

// mustImportWidgetType is mustSaveWidgetType in projectID, returning the type
// as the imported saved state holds it.
func mustImportWidgetType(t *testing.T, projectID id.ID[project.Project], ports ...library.InputPort) library.WidgetTypeCopy {
	t.Helper()
	repo := newWidgetTypeRepo(t)
	saved := mustStoreWidgetType(t, makeWidgetType(t, "type", ports, repo), repo)
	commit, err := mustCommit(t, repo.libraryID)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	mustImport(t, projectID, commit)
	return copyOfCommit(t, commit, saved.ID())
}

// cleanLibraries removes every import and library with its Draft and saved states.
func cleanLibraries(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(),
		"DELETE FROM engineering.widgets; DELETE FROM engineering.library_imports; DELETE FROM engineering.libraries"); err != nil {
		t.Fatalf("cleanLibraries: %v", err)
	}
}
