package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func TestLibrarySave_UpdateAndNameTaken(t *testing.T) {
	cleanLibraries(t)
	ctx := context.Background()
	repo := repositories.NewLibraryRepositoryPostgres(testDB)
	lib := mustSaveLibrary(t)
	other := mustSaveLibrary(t)

	if lib.Version() != version.Committed[library.Library]() {
		t.Errorf("a new library: version %s, want committed", lib.Version())
	}
	renamed, _ := lib.Update(lib.Version(), other.Name(), lib.Description())
	if _, err := repo.Save(ctx, renamed); !errors.Is(err, library.ErrLibraryNameTaken) {
		t.Errorf("a taken name: expected ErrLibraryNameTaken, got %v", err)
	}
	if _, err := repo.Save(ctx, lib); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := repo.Save(ctx, lib); !errors.Is(err, library.ErrLibraryConflict) {
		t.Errorf("a stale write: expected ErrLibraryConflict, got %v", err)
	}

	byName, err := repo.FindByName(ctx, soleOrganization(t), lib.Name())
	if err != nil || byName.ID() != lib.ID() {
		t.Errorf("FindByName: %v, %v", byName, err)
	}
	all, err := repo.FindAll(ctx, soleOrganization(t))
	if err != nil || len(all) != 2 || all[0].ID() != lib.ID() {
		t.Errorf("FindAll: %v, %v", all, err)
	}
}

// TestLibraryDelete_OnlyWhileNoLiveProjectPinsIt: marking a library deleted
// is refused while a live project pins it; the pin of a project marked
// deleted holds only the row's removal.
func TestLibraryDelete_OnlyWhileNoLiveProjectPinsIt(t *testing.T) {
	cleanProjects(t)
	cleanLibraries(t)
	ctx := context.Background()
	libraries := repositories.NewLibraryRepositoryPostgres(testDB)
	projects := repositories.NewProjectRepositoryPostgres(testDB)
	other := mustCreateProject(t, projects, "Line 1")
	wt := mustImportWidgetType(t, other.ID())
	lib, _ := libraries.FindByID(ctx, wt.LibraryID())

	if _, err := libraries.Save(ctx, lib.Delete()); !errors.Is(err, library.ErrLibraryImported) {
		t.Fatalf("pinned by a live project: expected ErrLibraryPinned, got %v", err)
	}
	if _, err := libraries.FindByID(ctx, lib.ID()); err != nil {
		t.Errorf("a refused delete must leave the library live: %v", err)
	}

	if _, err := projects.Save(ctx, other.Delete()); err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	if _, err := libraries.Save(ctx, lib.Delete()); err != nil {
		t.Fatalf("pinned by a deleted project only: %v", err)
	}
	if _, err := libraries.FindByID(ctx, lib.ID()); !errors.Is(err, library.ErrLibraryNotFound) {
		t.Errorf("a library marked deleted: expected ErrLibraryNotFound, got %v", err)
	}
	deleted, err := libraries.FindDeleted(ctx, lib.ID())
	if err != nil {
		t.Fatalf("FindDeleted: %v", err)
	}
	if err := libraries.Delete(ctx, deleted); !errors.Is(err, library.ErrLibraryImported) {
		t.Errorf("removal while a deleted project pins it: expected ErrLibraryPinned, got %v", err)
	}
	if _, err := testDB.ExecContext(ctx, `DELETE FROM engineering.library_imports`); err != nil {
		t.Fatal(err)
	}
	if err := libraries.Delete(ctx, deleted); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var left int
	if err := testDB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM engineering.widget_types WHERE library_id = $1)
	          + (SELECT count(*) FROM engineering.saved_states c JOIN engineering.drafts d ON d.id = c.draft_id
	             WHERE d.library_id = $1)`, lib.ID().UUID()).Scan(&left); err != nil || left != 0 {
		t.Errorf("rows of the removed library left: %d, %v", left, err)
	}
	if !errors.Is(libraries.Delete(ctx, deleted), library.ErrLibraryNotFound) {
		t.Error("a second removal: expected ErrLibraryNotFound")
	}
}
