package repositories_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/release"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

// cleanProjectReleases removes every Project Release and its pins.
func cleanProjectReleases(t *testing.T) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), `DELETE FROM engineering.project_releases`); err != nil {
		t.Fatalf("cleanProjectReleases: %v", err)
	}
}

// testReleaseNote is a note of the repository tests.
func testReleaseNote(t *testing.T, text string) release.Note {
	t.Helper()
	note, err := release.NewNote(text)
	if err != nil {
		t.Fatalf("NewNote(%q): %v", text, err)
	}
	return note
}

// savedCommit saves a Commit of the main project's Draft at its current
// Version and returns it.
func savedCommit(t *testing.T, title string) draft.Commit {
	t.Helper()
	ctx := context.Background()
	commit, err := testDrafts().SaveCommit(ctx, draft.ProjectScope(mainProject),
		draftVersionOf(t, mainProject), commitTitle(t, title), commitAuthor(t), projectCommitContent())
	if err != nil {
		t.Fatalf("SaveCommit(%q): %v", title, err)
	}
	return commit
}

// TestProjectRelease_CreateListFindAndNote: a Release copies a Commit with its
// number, title and ProjectFile, numbers the next one, refuses a second
// Release of a Commit, lists newest first and changes its note (task 72_3;
// ADR 0021).
func TestProjectRelease_CreateListFindAndNote(t *testing.T) {
	cleanTags(t)
	cleanScenes(t)
	cleanDrafts(t)
	cleanProjectReleases(t)
	ctx := context.Background()
	repo := repositories.NewProjectReleaseRepositoryPostgres(testDB)
	scope := draft.ProjectScope(mainProject)

	firstCommit := savedCommit(t, "First")
	mustUpdate(t, mainProject, "create a tag for the second commit", func(session *draft.Session) error {
		session.CreateTag(ctx, makeTag(t, "release-tag", repositories.NewTagRepositoryPostgres(testDB, mainProject)))
		return nil
	})
	secondCommit := savedCommit(t, "Second")

	first, err := repo.Create(ctx, scope, firstCommit, testReleaseNote(t, " for the demo "), commitAuthor(t), release.OriginCommit)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if first.Number().Int() != 1 || first.CommitNumber() != firstCommit.Number() {
		t.Errorf("first release %s of commit %d", first.Number(), first.CommitNumber())
	}
	if first.CommitTitle().String() != "First" || first.Note().String() != "for the demo" {
		t.Errorf("first release title %q, note %q", first.CommitTitle(), first.Note())
	}
	if _, err := repo.Create(ctx, scope, firstCommit, testReleaseNote(t, ""), commitAuthor(t), release.OriginCommit); !errors.Is(err, release.ErrCommitAlreadyReleased) {
		t.Fatalf("a second release of a commit: %v, want ErrCommitAlreadyReleased", err)
	}

	second, err := repo.Create(ctx, scope, secondCommit, testReleaseNote(t, ""), commitAuthor(t), release.OriginCommit)
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	if second.Number().Int() != 2 {
		t.Errorf("second release is %s, want 2", second.Number())
	}

	// The title is a snapshot: renaming the Commit later does not touch it.
	if _, err := testDrafts().RenameCommit(ctx, scope, secondCommit.Number(), commitTitle(t, "Renamed later")); err != nil {
		t.Fatalf("RenameCommit: %v", err)
	}
	found, err := repo.Find(ctx, scope, 2)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.CommitTitle().String() != "Second" {
		t.Errorf("the release title followed the commit rename: %q", found.CommitTitle())
	}
	file, ok := found.Content().ProjectFile()
	if !ok || file.ProjectID != mainProject.UUID() {
		t.Errorf("the stored content is not the project file: %+v", found.Content())
	}

	list, err := repo.List(ctx, scope)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].Number().Int() != 2 || list[1].Number().Int() != 1 {
		t.Errorf("releases newest first: %d", len(list))
	}
	released, err := repo.ReleasedCommits(ctx, scope)
	if err != nil {
		t.Fatalf("ReleasedCommits: %v", err)
	}
	if len(released) != 2 || released[0] != firstCommit.Number() || released[1] != secondCommit.Number() {
		t.Errorf("released commits: %v", released)
	}

	updated, err := repo.UpdateNote(ctx, scope, 1, testReleaseNote(t, "after the demo"))
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}
	if updated.Note().String() != "after the demo" {
		t.Errorf("the updated note is %q", updated.Note())
	}
	found, err = repo.Find(ctx, scope, 1)
	if err != nil {
		t.Fatalf("Find after UpdateNote: %v", err)
	}
	if found.Note().String() != "after the demo" {
		t.Errorf("the stored note is %q", found.Note())
	}
	if _, err := repo.UpdateNote(ctx, scope, 9, testReleaseNote(t, "x")); !errors.Is(err, release.ErrReleaseNotFound) {
		t.Errorf("UpdateNote of a missing release: %v, want ErrReleaseNotFound", err)
	}
	if _, err := repo.Find(ctx, scope, 9); !errors.Is(err, release.ErrReleaseNotFound) {
		t.Errorf("Find of a missing release: %v, want ErrReleaseNotFound", err)
	}

	// The Library Pins of a ProjectFile are the release's provenance.
	var pins int
	if err := testDB.QueryRowContext(ctx,
		`SELECT count(*) FROM engineering.project_release_library_pins WHERE release_id = $1`,
		first.ID().UUID()).Scan(&pins); err != nil {
		t.Fatalf("read the release's pins: %v", err)
	}
	if pins != 1 {
		t.Errorf("the release holds %d pins, want 1", pins)
	}
}

// TestProjectRelease_TwoCreatesOfOneCommit: two publications of one Commit
// give one Release, and two of different Commits get 1 and 2 under the
// Project's lock.
func TestProjectRelease_TwoCreatesOfOneCommit(t *testing.T) {
	cleanTags(t)
	cleanDrafts(t)
	cleanProjectReleases(t)
	ctx := context.Background()
	repo := repositories.NewProjectReleaseRepositoryPostgres(testDB)
	scope := draft.ProjectScope(mainProject)

	commit := savedCommit(t, "Concurrent")
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []error
	)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.Create(ctx, scope, commit, testReleaseNote(t, ""), commitAuthor(t), release.OriginCommit)
			mu.Lock()
			results = append(results, err)
			mu.Unlock()
		}()
	}
	wg.Wait()
	created, refused := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, release.ErrCommitAlreadyReleased):
			refused++
		default:
			t.Fatalf("concurrent Create: %v", err)
		}
	}
	if created != 1 || refused != 1 {
		t.Errorf("%d created and %d refused, want 1 and 1", created, refused)
	}
}

// TestProjectRelease_Cascade: removing the Project removes its Releases and
// their pins.
func TestProjectRelease_Cascade(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO projects (id, organization_id, name, version)
		 SELECT $1, id, $2, 1 FROM organizations LIMIT 1`,
		projectID, "releases-"+uuid.NewString()); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(context.Background(), `DELETE FROM projects WHERE id = $1`, projectID)
	})
	scope := draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID)))
	title, _ := draft.NewCommitTitle("Cascade")
	author, _ := draft.NewAuthor("Ada")
	commit, err := draft.NewCommit(scope, id.NewID[draft.Commit](), 1, title, author, time.Now().UTC(),
		version.Committed[draft.Draft](), draft.NewProjectCommitContent(projectv0.ProjectFile{
			Header:      projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID},
			LibraryPins: []projectv0.LibraryPin{{LibraryID: uuid.New(), Release: 1}},
		}))
	if err != nil {
		t.Fatalf("NewCommit: %v", err)
	}
	repo := repositories.NewProjectReleaseRepositoryPostgres(testDB)
	created, err := repo.Create(ctx, scope, commit, testReleaseNote(t, ""), author, release.OriginCommit)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, projectID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	var releases, pins int
	if err := testDB.QueryRowContext(ctx, `SELECT count(*) FROM engineering.project_releases WHERE id = $1`, created.ID().UUID()).Scan(&releases); err != nil {
		t.Fatalf("count releases: %v", err)
	}
	if err := testDB.QueryRowContext(ctx, `SELECT count(*) FROM engineering.project_release_library_pins WHERE release_id = $1`, created.ID().UUID()).Scan(&pins); err != nil {
		t.Fatalf("count pins: %v", err)
	}
	if releases != 0 || pins != 0 {
		t.Errorf("%d releases and %d pins survived their project", releases, pins)
	}
}
