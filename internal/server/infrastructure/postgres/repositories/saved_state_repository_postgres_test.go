package repositories_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// savedStateTitle is a title of the repository tests.
func savedStateTitle(t *testing.T, text string) draft.SavedStateTitle {
	t.Helper()
	title, err := draft.NewSavedStateTitle(text)
	if err != nil {
		t.Fatalf("NewSavedStateTitle(%q): %v", text, err)
	}
	return title
}

// savedStateAuthor is the author of the repository tests.
func savedStateAuthor(t *testing.T) draft.Author {
	t.Helper()
	author, err := draft.NewAuthor("Ada")
	if err != nil {
		t.Fatalf("NewAuthor: %v", err)
	}
	return author
}

// projectSavedStateContent is a canonical-enough ProjectFile of the main
// project, with one import to check that a Project's SavedState stores them.
func projectSavedStateContent() draft.SavedStateContent {
	return draft.NewProjectSavedStateContent(projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: mainProject.UUID()},
		Tags: []projectv0.TagDefinition{
			{ID: uuid.New(), Name: "speed", Type: "integer"},
		},
		Scenes: []projectv0.Scene{
			{ID: uuid.New(), Name: "Main", Width: 100, Height: 100, Widgets: []projectv0.Widget{}},
		},
		LibraryImports: []projectv0.LibraryImport{{LibraryID: uuid.New(), Commit: 3}},
	})
}

// TestDraftSaveSavedState_NumbersAndContent: a Save fixes the Draft as
// SavedState 1, then 2, records what the last one fixed, and stores the
// snapshot with the imports of a Project's ProjectFile (task 72_2; ADR 0021).
func TestDraftSaveSavedState_NumbersAndContent(t *testing.T) {
	cleanTags(t)
	cleanScenes(t)
	cleanDrafts(t)
	ctx := context.Background()
	scope := draft.ProjectScope(mainProject)
	content := projectSavedStateContent()
	tags := repositories.NewTagRepositoryPostgres(testDB, mainProject)

	first, err := testDrafts().SaveSavedState(ctx, scope, draftVersionOf(t, mainProject), savedStateTitle(t, "First"), savedStateAuthor(t), content)
	if err != nil {
		t.Fatalf("SaveSavedState: %v", err)
	}
	if first.Number() != 1 || first.Title().String() != "First" || first.Author().String() != "Ada" {
		t.Errorf("first savedState: %s", first)
	}
	if first.DraftVersion().Number() != draftVersionOf(t, mainProject).Number() {
		t.Errorf("saved state fixed version %s, want %s", first.DraftVersion(), draftVersionOf(t, mainProject))
	}
	if _, err := testDrafts().SaveSavedState(ctx, scope, draftVersionOf(t, mainProject), savedStateTitle(t, "Second"), savedStateAuthor(t), content); !errors.Is(err, draft.ErrNothingToSave) {
		t.Fatalf("SaveSavedState without a change: %v, want ErrNothingToSave", err)
	}

	// A write raises the Version, so the next Save is a new SavedState.
	mustUpdate(t, mainProject, "create a tag", func(session *draft.Session) error {
		session.CreateTag(ctx, makeTag(t, "speed-for-savedState", tags))
		return nil
	})
	second, err := testDrafts().SaveSavedState(ctx, scope, draftVersionOf(t, mainProject), savedStateTitle(t, "Second"), savedStateAuthor(t), content)
	if err != nil {
		t.Fatalf("second SaveSavedState: %v", err)
	}
	if second.Number() != 2 {
		t.Errorf("the second saved state is %d, want 2", second.Number())
	}

	var lastNumber, lastVersion int
	if err := testDB.QueryRowContext(ctx,
		`SELECT last_saved_state_number, last_saved_state_draft_version FROM engineering.drafts WHERE project_id = $1`,
		mainProject.UUID()).Scan(&lastNumber, &lastVersion); err != nil {
		t.Fatalf("read the draft's last savedState: %v", err)
	}
	if lastNumber != 2 || lastVersion != draftVersionOf(t, mainProject).Number() {
		t.Errorf("last saved state %d at version %d, want 2 at %d", lastNumber, lastVersion, draftVersionOf(t, mainProject).Number())
	}

	var imports int
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM engineering.saved_state_library_imports WHERE saved_state_id = $1`, second.ID().UUID()).Scan(&imports); err != nil {
		t.Fatalf("read the savedState's imports: %v", err)
	}
	if imports != 1 {
		t.Errorf("the project's saved state holds %d imports, want 1", imports)
	}

	found, err := testDrafts().FindSavedState(ctx, scope, 2)
	if err != nil {
		t.Fatalf("FindSavedState: %v", err)
	}
	if file, ok := found.Content().ProjectFile(); !ok || file.ProjectID != mainProject.UUID() {
		t.Errorf("the stored content is not the project file: %+v", found.Content())
	}
	list, err := testDrafts().FindSavedStates(ctx, scope, draft.SavedStateFilter{})
	if err != nil {
		t.Fatalf("FindSavedStates: %v", err)
	}
	if len(list) != 2 || list[0].Number() != 2 || list[1].Number() != 1 {
		t.Errorf("savedStates newest first: %d", len(list))
	}
	lastSavedState, hasLast, err := testDrafts().FindLastSavedState(ctx, scope)
	if err != nil {
		t.Fatalf("FindLastSavedState: %v", err)
	}
	if !hasLast || lastSavedState.Number() != 2 {
		t.Errorf("the last savedState: %s, %t, want 2", lastSavedState, hasLast)
	}
}

// TestDraftSaveSavedState_StaleVersionAndTwoSaves: a Save of a Version a write
// has passed is a conflict, and two Saves of one Version give one SavedState.
func TestDraftSaveSavedState_StaleVersionAndTwoSaves(t *testing.T) {
	cleanTags(t)
	cleanDrafts(t)
	ctx := context.Background()
	scope := draft.ProjectScope(mainProject)
	content := projectSavedStateContent()
	tags := repositories.NewTagRepositoryPostgres(testDB, mainProject)

	if _, found, err := testDrafts().FindLastSavedState(ctx, scope); err != nil || found {
		t.Fatalf("FindLastSavedState of a Draft never saved: %t, %v, want none", found, err)
	}

	read := draftVersionOf(t, mainProject)
	mustUpdate(t, mainProject, "create a tag", func(session *draft.Session) error {
		session.CreateTag(ctx, makeTag(t, "stale-savedState-tag", tags))
		return nil
	})
	if _, err := testDrafts().SaveSavedState(ctx, scope, read, savedStateTitle(t, "Stale"), savedStateAuthor(t), content); !errors.Is(err, draft.ErrDraftConflict) {
		t.Fatalf("SaveSavedState at a stale version: %v, want ErrDraftConflict", err)
	}

	version := draftVersionOf(t, mainProject)
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []error
	)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := testDrafts().SaveSavedState(ctx, scope, version, savedStateTitle(t, "Concurrent"), savedStateAuthor(t), content)
			mu.Lock()
			results = append(results, err)
			mu.Unlock()
		}()
	}
	wg.Wait()
	saved := 0
	for _, err := range results {
		if err == nil {
			saved++
		}
	}
	if saved != 1 {
		t.Fatalf("%d saves of one version succeeded, want 1: %v", saved, results)
	}
}

// TestDraftSaveSavedState_RenameAndCascade: a title changes without a Draft
// Version, and removing the Draft removes its saved states (task 72_2;
// ADR 0021).
func TestDraftSaveSavedState_RenameAndCascade(t *testing.T) {
	cleanDrafts(t)
	ctx := context.Background()
	scope := draft.ProjectScope(mainProject)
	savedState, err := testDrafts().SaveSavedState(ctx, scope, draftVersionOf(t, mainProject), savedStateTitle(t, "Before"), savedStateAuthor(t), projectSavedStateContent())
	if err != nil {
		t.Fatalf("SaveSavedState: %v", err)
	}
	renamed, err := testDrafts().RenameSavedState(ctx, scope, savedState.Number(), savedStateTitle(t, "After"))
	if err != nil {
		t.Fatalf("RenameSavedState: %v", err)
	}
	if renamed.Title().String() != "After" {
		t.Errorf("renamed title %q", renamed.Title())
	}
	found, err := testDrafts().FindSavedState(ctx, scope, savedState.Number())
	if err != nil {
		t.Fatalf("FindSavedState after rename: %v", err)
	}
	if found.Title().String() != "After" {
		t.Errorf("stored title %q", found.Title())
	}
	if _, err := testDrafts().RenameSavedState(ctx, scope, 99, savedStateTitle(t, "Missing")); !errors.Is(err, draft.ErrSavedStateNotFound) {
		t.Errorf("RenameSavedState of a missing savedState: %v, want ErrSavedStateNotFound", err)
	}

	if _, err := testDB.ExecContext(ctx, `DELETE FROM engineering.drafts WHERE project_id = $1`, mainProject.UUID()); err != nil {
		t.Fatalf("delete the draft: %v", err)
	}
	var count int
	if err := testDB.QueryRowContext(ctx, `SELECT count(*) FROM engineering.saved_states`).Scan(&count); err != nil {
		t.Fatalf("count savedStates: %v", err)
	}
	if count != 0 {
		t.Errorf("%d savedStates survived their draft", count)
	}
}

// TestDraftSaveSavedState_Library: a Library's SavedState holds the copies of
// its WidgetTypes, encoded with the format version and read back (task 72_2;
// ADR 0021).
func TestDraftSaveSavedState_Library(t *testing.T) {
	cleanWidgetTypes(t)
	ctx := context.Background()
	repo := newWidgetTypeRepo(t)
	wt := mustCreateWidgetType(t, repo.libraryID, makeWidgetType(t, "gauge", nil, repo))
	scope := draft.LibraryScope(repo.libraryID)
	content := draft.NewLibrarySavedStateContent([]library.WidgetTypeCopy{library.CopyOf(wt)})

	saved, err := testDrafts().SaveSavedState(ctx, scope, libraryDraftVersionOf(t, repo.libraryID), savedStateTitle(t, "Widgets"), savedStateAuthor(t), content)
	if err != nil {
		t.Fatalf("SaveSavedState: %v", err)
	}
	found, err := testDrafts().FindSavedState(ctx, scope, saved.Number())
	if err != nil {
		t.Fatalf("FindSavedState: %v", err)
	}
	types, ok := found.Content().WidgetTypes()
	if !ok || len(types) != 1 || types[0].ID() != wt.ID() {
		t.Errorf("the library's saved state holds %d widget types, want %s", len(types), wt.ID())
	}
	savedStates, err := testDrafts().FindSavedStates(ctx, scope, draft.SavedStateFilter{})
	if err != nil {
		t.Fatalf("FindSavedStates: %v", err)
	}
	if len(savedStates) != 1 {
		t.Errorf("%d savedStates of the library, want 1", len(savedStates))
	}
}
