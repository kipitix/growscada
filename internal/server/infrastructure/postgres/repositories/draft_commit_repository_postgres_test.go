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

// commitTitle is a title of the repository tests.
func commitTitle(t *testing.T, text string) draft.CommitTitle {
	t.Helper()
	title, err := draft.NewCommitTitle(text)
	if err != nil {
		t.Fatalf("NewCommitTitle(%q): %v", text, err)
	}
	return title
}

// commitAuthor is the author of the repository tests.
func commitAuthor(t *testing.T) draft.Author {
	t.Helper()
	author, err := draft.NewAuthor("Ada")
	if err != nil {
		t.Fatalf("NewAuthor: %v", err)
	}
	return author
}

// projectCommitContent is a canonical-enough ProjectFile of the main project,
// with one pin to check that a Project's Commit stores them.
func projectCommitContent() draft.CommitContent {
	return draft.NewProjectCommitContent(projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: mainProject.UUID()},
		Tags: []projectv0.TagDefinition{
			{ID: uuid.New(), Name: "speed", Type: "integer"},
		},
		Scenes: []projectv0.Scene{
			{ID: uuid.New(), Name: "Main", Width: 100, Height: 100, Widgets: []projectv0.Widget{}},
		},
		LibraryPins: []projectv0.LibraryPin{{LibraryID: uuid.New(), Release: 3}},
	})
}

// TestDraftSaveCommit_NumbersAndContent: a Save fixes the Draft as Commit 1,
// then 2, records what the last one fixed, and stores the snapshot with the
// pins of a Project's ProjectFile (task 72_2; ADR 0021).
func TestDraftSaveCommit_NumbersAndContent(t *testing.T) {
	cleanTags(t)
	cleanScenes(t)
	cleanDrafts(t)
	ctx := context.Background()
	scope := draft.ProjectScope(mainProject)
	content := projectCommitContent()
	tags := repositories.NewTagRepositoryPostgres(testDB, mainProject)

	first, err := testDrafts().SaveCommit(ctx, scope, draftVersionOf(t, mainProject), commitTitle(t, "First"), commitAuthor(t), content)
	if err != nil {
		t.Fatalf("SaveCommit: %v", err)
	}
	if first.Number() != 1 || first.Title().String() != "First" || first.Author().String() != "Ada" {
		t.Errorf("first commit: %s", first)
	}
	if first.DraftVersion().Number() != draftVersionOf(t, mainProject).Number() {
		t.Errorf("commit fixed version %s, want %s", first.DraftVersion(), draftVersionOf(t, mainProject))
	}
	if _, err := testDrafts().SaveCommit(ctx, scope, draftVersionOf(t, mainProject), commitTitle(t, "Second"), commitAuthor(t), content); !errors.Is(err, draft.ErrNothingToSave) {
		t.Fatalf("SaveCommit without a change: %v, want ErrNothingToSave", err)
	}

	// A write raises the Version, so the next Save is a new Commit.
	mustUpdate(t, mainProject, "create a tag", func(session *draft.Session) error {
		session.CreateTag(ctx, makeTag(t, "speed-for-commit", tags))
		return nil
	})
	second, err := testDrafts().SaveCommit(ctx, scope, draftVersionOf(t, mainProject), commitTitle(t, "Second"), commitAuthor(t), content)
	if err != nil {
		t.Fatalf("second SaveCommit: %v", err)
	}
	if second.Number() != 2 {
		t.Errorf("second commit is %d, want 2", second.Number())
	}

	var lastNumber, lastVersion int
	if err := testDB.QueryRowContext(ctx,
		`SELECT last_commit_number, last_commit_draft_version FROM engineering.drafts WHERE project_id = $1`,
		mainProject.UUID()).Scan(&lastNumber, &lastVersion); err != nil {
		t.Fatalf("read the draft's last commit: %v", err)
	}
	if lastNumber != 2 || lastVersion != draftVersionOf(t, mainProject).Number() {
		t.Errorf("last commit %d at version %d, want 2 at %d", lastNumber, lastVersion, draftVersionOf(t, mainProject).Number())
	}

	var pins int
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM engineering.commit_library_pins WHERE commit_id = $1`, second.ID().UUID()).Scan(&pins); err != nil {
		t.Fatalf("read the commit's pins: %v", err)
	}
	if pins != 1 {
		t.Errorf("the project's commit holds %d pins, want 1", pins)
	}

	found, err := testDrafts().FindCommit(ctx, scope, 2)
	if err != nil {
		t.Fatalf("FindCommit: %v", err)
	}
	if file, ok := found.Content().ProjectFile(); !ok || file.ProjectID != mainProject.UUID() {
		t.Errorf("the stored content is not the project file: %+v", found.Content())
	}
	list, err := testDrafts().FindCommits(ctx, scope)
	if err != nil {
		t.Fatalf("FindCommits: %v", err)
	}
	if len(list) != 2 || list[0].Number() != 2 || list[1].Number() != 1 {
		t.Errorf("commits newest first: %d", len(list))
	}
	lastCommit, hasLast, err := testDrafts().FindLastCommit(ctx, scope)
	if err != nil {
		t.Fatalf("FindLastCommit: %v", err)
	}
	if !hasLast || lastCommit.Number() != 2 {
		t.Errorf("the last commit: %s, %t, want 2", lastCommit, hasLast)
	}
}

// TestDraftSaveCommit_StaleVersionAndTwoSaves: a Save of a Version a write has
// passed is a conflict, and two Saves of one Version give one Commit.
func TestDraftSaveCommit_StaleVersionAndTwoSaves(t *testing.T) {
	cleanTags(t)
	cleanDrafts(t)
	ctx := context.Background()
	scope := draft.ProjectScope(mainProject)
	content := projectCommitContent()
	tags := repositories.NewTagRepositoryPostgres(testDB, mainProject)

	if _, found, err := testDrafts().FindLastCommit(ctx, scope); err != nil || found {
		t.Fatalf("FindLastCommit of a Draft never saved: %t, %v, want none", found, err)
	}

	read := draftVersionOf(t, mainProject)
	mustUpdate(t, mainProject, "create a tag", func(session *draft.Session) error {
		session.CreateTag(ctx, makeTag(t, "stale-commit-tag", tags))
		return nil
	})
	if _, err := testDrafts().SaveCommit(ctx, scope, read, commitTitle(t, "Stale"), commitAuthor(t), content); !errors.Is(err, draft.ErrDraftConflict) {
		t.Fatalf("SaveCommit at a stale version: %v, want ErrDraftConflict", err)
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
			_, err := testDrafts().SaveCommit(ctx, scope, version, commitTitle(t, "Concurrent"), commitAuthor(t), content)
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

// TestDraftSaveCommit_RenameAndCascade: a title changes without a Draft
// Version, and removing the Draft removes its Commits (task 72_2;
// ADR 0021).
func TestDraftSaveCommit_RenameAndCascade(t *testing.T) {
	cleanDrafts(t)
	ctx := context.Background()
	scope := draft.ProjectScope(mainProject)
	commit, err := testDrafts().SaveCommit(ctx, scope, draftVersionOf(t, mainProject), commitTitle(t, "Before"), commitAuthor(t), projectCommitContent())
	if err != nil {
		t.Fatalf("SaveCommit: %v", err)
	}
	renamed, err := testDrafts().RenameCommit(ctx, scope, commit.Number(), commitTitle(t, "After"))
	if err != nil {
		t.Fatalf("RenameCommit: %v", err)
	}
	if renamed.Title().String() != "After" {
		t.Errorf("renamed title %q", renamed.Title())
	}
	found, err := testDrafts().FindCommit(ctx, scope, commit.Number())
	if err != nil {
		t.Fatalf("FindCommit after rename: %v", err)
	}
	if found.Title().String() != "After" {
		t.Errorf("stored title %q", found.Title())
	}
	if _, err := testDrafts().RenameCommit(ctx, scope, 99, commitTitle(t, "Missing")); !errors.Is(err, draft.ErrCommitNotFound) {
		t.Errorf("RenameCommit of a missing commit: %v, want ErrCommitNotFound", err)
	}

	if _, err := testDB.ExecContext(ctx, `DELETE FROM engineering.drafts WHERE project_id = $1`, mainProject.UUID()); err != nil {
		t.Fatalf("delete the draft: %v", err)
	}
	var count int
	if err := testDB.QueryRowContext(ctx, `SELECT count(*) FROM engineering.commits`).Scan(&count); err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if count != 0 {
		t.Errorf("%d commits survived their draft", count)
	}
}

// TestDraftSaveCommit_Library: a Library's Commit holds the copies of its
// WidgetTypes, encoded with the format version and read back (task 72_2;
// ADR 0021).
func TestDraftSaveCommit_Library(t *testing.T) {
	cleanWidgetTypes(t)
	ctx := context.Background()
	repo := newWidgetTypeRepo(t)
	wt := mustCreateWidgetType(t, repo.libraryID, makeWidgetType(t, "gauge", nil, repo))
	scope := draft.LibraryScope(repo.libraryID)
	content := draft.NewLibraryCommitContent([]library.ReleasedWidgetType{library.ReleasedFrom(wt)})

	saved, err := testDrafts().SaveCommit(ctx, scope, libraryDraftVersionOf(t, repo.libraryID), commitTitle(t, "Widgets"), commitAuthor(t), content)
	if err != nil {
		t.Fatalf("SaveCommit: %v", err)
	}
	found, err := testDrafts().FindCommit(ctx, scope, saved.Number())
	if err != nil {
		t.Fatalf("FindCommit: %v", err)
	}
	types, ok := found.Content().WidgetTypes()
	if !ok || len(types) != 1 || types[0].ID() != wt.ID() {
		t.Errorf("the library's commit holds %d widget types, want %s", len(types), wt.ID())
	}
	commits, err := testDrafts().FindCommits(ctx, scope)
	if err != nil {
		t.Fatalf("FindCommits: %v", err)
	}
	if len(commits) != 1 {
		t.Errorf("%d commits of the library, want 1", len(commits))
	}
}
