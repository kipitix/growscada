package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/libraryimport"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// TestLibraryImportRepository_ReadsTheSavedStateAnImportNames: an import
// written by the Draft carries the identity of its own Library's saved state —
// two Libraries at saved state 1 are told apart — and the WidgetTypes the
// Project may be of come from those saved states' content (task 85_6).
func TestLibraryImportRepository_ReadsTheSavedStateAnImportNames(t *testing.T) {
	cleanLibraries(t)
	first := mustImportWidgetType(t, mainProject)
	second := mustImportWidgetType(t, mainProject)
	ctx := context.Background()
	imports := repositories.NewLibraryImportRepositoryPostgres(testDB, mainProject)

	all, err := imports.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("the imports: %d, want the two libraries", len(all))
	}
	for _, imported := range all {
		savedState, err := testDrafts().FindSavedState(ctx, draft.LibraryScope(imported.LibraryID()), imported.SavedStateNumber())
		if err != nil {
			t.Fatalf("FindSavedState of library %s: %v", imported.LibraryID(), err)
		}
		if imported.SavedStateID() != savedState.ID().UUID() {
			t.Errorf("the import of library %s names saved state %s of the saved state, not %s",
				imported.LibraryID(), imported.SavedStateID(), savedState.ID())
		}
	}

	// The palette comes from the imported saved states, and only from them.
	types, err := imports.FindImportedWidgetTypes(ctx)
	if err != nil {
		t.Fatalf("FindImportedWidgetTypes: %v", err)
	}
	if len(types) != 2 {
		t.Errorf("the imported widget types: %+v, want both", types)
	}
	found, err := imports.FindImportedWidgetType(ctx, second.ID())
	if err != nil || found.ID() != second.ID() || found.LibraryID() != second.LibraryID() {
		t.Errorf("FindImportedWidgetType: %+v, %v", found, err)
	}
	if _, err := imports.FindImportedWidgetType(ctx, first.ID()); err != nil {
		t.Errorf("the other library's type: %v", err)
	}
	missing := id.NewID[library.WidgetType]()
	if _, err := imports.FindImportedWidgetType(ctx, missing); !errors.Is(err, library.ErrWidgetTypeNotFound) {
		t.Errorf("a type no saved state holds: expected ErrWidgetTypeNotFound, got %v", err)
	}
	if _, err := imports.FindByLibrary(ctx, id.NewID[library.Library]()); !errors.Is(err, libraryimport.ErrLibraryImportNotFound) {
		t.Errorf("a library the project does not import: expected ErrLibraryImportNotFound, got %v", err)
	}

	// The row holds its saved state: a SavedState an import names cannot be
	// removed on its own (fk_library_imports_saved_state_id).
	for _, imported := range all {
		if _, err := testDB.ExecContext(ctx,
			`DELETE FROM engineering.saved_states WHERE id = $1`, imported.SavedStateID()); err == nil {
			t.Errorf("the saved state of the import of library %s was removed under it", imported.LibraryID())
		}
	}
}
