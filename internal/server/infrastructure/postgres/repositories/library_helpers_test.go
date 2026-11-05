package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
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
	saved, err := repo.Save(editlock.System(context.Background()), library.CreateLibrary(repo.NextID(), soleOrganization(t), name))
	if err != nil {
		t.Fatalf("Save library: %v", err)
	}
	return saved
}

// cleanWidgetTypes deletes the libraries, with their Drafts and releases.
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

// mustRelease releases the Draft of the library as it is.
func mustRelease(t *testing.T, libraryID id.ID[library.Library]) library.LibraryRelease {
	t.Helper()
	ctx := editlock.System(context.Background())
	lib, err := repositories.NewLibraryRepositoryPostgres(testDB).FindByID(ctx, libraryID)
	if err != nil {
		t.Fatalf("FindByID library: %v", err)
	}
	draft, err := repositories.NewWidgetTypeRepositoryPostgres(testDB, libraryID).FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll widget types: %v", err)
	}
	releases := repositories.NewLibraryReleaseRepositoryPostgres(testDB)
	latest, err := releases.FindLatest(ctx, libraryID)
	if err != nil {
		t.Fatalf("FindLatest: %v", err)
	}
	release, err := library.CreateRelease(lib, draft, latest, "", time.Now())
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	saved, err := releases.Save(ctx, release, libraryDraftContentVersionOf(t, libraryID))
	if err != nil {
		t.Fatalf("Save release: %v", err)
	}
	return saved
}

// mustPin pins release in the project through the project's Draft.
func mustPin(t *testing.T, projectID id.ID[project.Project], release library.LibraryRelease) librarypin.LibraryPin {
	t.Helper()
	ctx := editlock.System(context.Background())
	var saved librarypin.LibraryPin
	mustUpdate(t, projectID, "test pin", func(session *draft.Session) error {
		var err error
		saved, _, err = session.PutLibraryPin(ctx, release.LibraryID(), release.Number())
		return err
	})
	return saved
}

// mustSaveWidgetType stores a new WidgetType with the given ports in a new
// library, releases it and pins the release in the project Main: a Widget's
// type must be pinned.
func mustSaveWidgetType(t *testing.T, ports ...library.InputPort) id.ID[library.WidgetType] {
	t.Helper()
	return mustPinWidgetType(t, mainProject, ports...).ID()
}

// mustPinWidgetType is mustSaveWidgetType in projectID, returning the type
// as released.
func mustPinWidgetType(t *testing.T, projectID id.ID[project.Project], ports ...library.InputPort) library.ReleasedWidgetType {
	t.Helper()
	repo := newWidgetTypeRepo(t)
	saved := mustStoreWidgetType(t, makeWidgetType(t, "type", ports, repo), repo)
	release := mustRelease(t, repo.libraryID)
	mustPin(t, projectID, release)
	released, _ := release.FindWidgetType(saved.ID())
	return released
}

// cleanLibraries removes every pin and library with its Draft and releases.
func cleanLibraries(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(editlock.System(context.Background()),
		"DELETE FROM engineering.widgets; DELETE FROM engineering.library_pins; DELETE FROM engineering.libraries"); err != nil {
		t.Fatalf("cleanLibraries: %v", err)
	}
}
