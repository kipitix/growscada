package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func TestLibrarySave_RenameAndNameTaken(t *testing.T) {
	cleanLibraries(t)
	ctx := context.Background()
	repo := repositories.NewLibraryRepositoryPostgres(testDB)
	lib := mustSaveLibrary(t)
	other := mustSaveLibrary(t)

	if lib.Version() != version.Committed[library.Library]() {
		t.Errorf("a new library: version %s, want committed", lib.Version())
	}
	renamed, _ := lib.Rename(lib.Version(), other.Name())
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
	wt := mustPinWidgetType(t, other.ID())
	lib, _ := libraries.FindByID(ctx, wt.LibraryID())

	if _, err := libraries.Save(ctx, lib.Delete()); !errors.Is(err, library.ErrLibraryPinned) {
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
	if err := libraries.Delete(ctx, deleted); !errors.Is(err, library.ErrLibraryPinned) {
		t.Errorf("removal while a deleted project pins it: expected ErrLibraryPinned, got %v", err)
	}
	if _, err := testDB.ExecContext(ctx, `DELETE FROM engineering.library_pins`); err != nil {
		t.Fatal(err)
	}
	if err := libraries.Delete(ctx, deleted); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var left int
	if err := testDB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM engineering.widget_types WHERE library_id = $1)
	          + (SELECT count(*) FROM engineering.library_releases WHERE library_id = $1)
	          + (SELECT count(*) FROM engineering.released_widget_types WHERE library_id = $1)`, lib.ID().UUID()).Scan(&left); err != nil || left != 0 {
		t.Errorf("rows of the removed library left: %d, %v", left, err)
	}
	if !errors.Is(libraries.Delete(ctx, deleted), library.ErrLibraryNotFound) {
		t.Error("a second removal: expected ErrLibraryNotFound")
	}
}

func TestLibraryRelease_SaveAndRead(t *testing.T) {
	cleanLibraries(t)
	ctx := context.Background()
	repo := newWidgetTypeRepo(t)
	port, _ := library.NewInputPortName("value")
	saved := mustStoreWidgetType(t, makeWidgetType(t, "gauge", []library.InputPort{library.NewInputPort(port, "the value", library.AnyTypeHint())}, repo), repo)
	releases := repositories.NewLibraryReleaseRepositoryPostgres(testDB)
	if latest, err := releases.FindLatest(ctx, repo.libraryID); err != nil || latest != nil {
		t.Fatalf("no release yet: %v, %v", latest, err)
	}
	first := mustRelease(t, repo.libraryID)
	if first.Number() != library.FirstRelease() {
		t.Errorf("first release numbered %s", first.Number())
	}
	commitNumber := first.CommitNumber()
	if commitNumber == nil || *commitNumber != 1 || first.Author().String() != "Ada" || first.Origin() != library.OriginCommit {
		t.Errorf("the provenance of the first release: %+v", first)
	}

	got, err := releases.FindByNumber(ctx, repo.libraryID, first.Number())
	if err != nil {
		t.Fatalf("FindByNumber: %v", err)
	}
	released, ok := got.FindWidgetType(saved.ID())
	if !ok || !released.Matches(saved) || got.CreatedAt().IsZero() {
		t.Errorf("the release read back: %s, %v", got, got.WidgetTypes())
	}
	if !got.CreatedAt().Equal(first.CreatedAt()) {
		t.Errorf("created at %v read back, %v saved", got.CreatedAt(), first.CreatedAt())
	}
	if _, err := releases.FindByNumber(ctx, repo.libraryID, first.Number().Next()); !errors.Is(err, library.ErrReleaseNotFound) {
		t.Errorf("a missing release: expected ErrReleaseNotFound, got %v", err)
	}
	if numbers, err := releases.ReleasedCommits(ctx, repo.libraryID); err != nil || len(numbers) != 1 || numbers[0] != 1 {
		t.Errorf("ReleasedCommits: %v, %v", numbers, err)
	}

	// One Commit is published at most once: a second Release of it is refused,
	// even under another number.
	lib, _ := repositories.NewLibraryRepositoryPostgres(testDB).FindByID(ctx, repo.libraryID)
	author, _ := library.NewAuthor("Ada")
	again, err := library.CreateRelease(lib, first.Number().Next(), *commitNumber, "", author, time.Now(), nil)
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if _, err := releases.Save(ctx, again); !errors.Is(err, library.ErrCommitAlreadyReleased) {
		t.Errorf("releasing commit 1 twice: expected ErrCommitAlreadyReleased, got %v", err)
	}

	if _, err := testDrafts().Update(ctx, draft.LibraryScope(repo.libraryID), libraryDraftVersionOf(t, repo.libraryID),
		"Delete widget type gauge", func(session *draft.Session) error {
			_, err := session.DeleteWidgetType(ctx, saved.ID())
			return err
		}); err != nil {
		t.Fatalf("Delete widget type: %v", err)
	}
	second := mustRelease(t, repo.libraryID)
	if second.Number().Int() != 2 {
		t.Fatalf("second release numbered %s", second.Number())
	}
	if numbers, err := releases.ReleasedCommits(ctx, repo.libraryID); err != nil || len(numbers) != 2 {
		t.Errorf("ReleasedCommits: %v, %v", numbers, err)
	}
	all, err := releases.FindAll(ctx, repo.libraryID)
	if err != nil || len(all) != 2 || len(all[1].WidgetTypes()) != 0 || len(all[0].WidgetTypes()) != 1 {
		t.Errorf("FindAll: %v, %v", all, err)
	}
	if latest, _ := releases.FindLatest(ctx, repo.libraryID); latest.Number().Int() != 2 {
		t.Errorf("FindLatest: %s", latest)
	}
}

// TestLibraryRelease_NoteAndLegacy: a note is metadata and changes alone,
// recording its event; a Release stored before Commits reads back as legacy,
// with its system author and no Commit.
func TestLibraryRelease_NoteAndLegacy(t *testing.T) {
	cleanLibraries(t)
	ctx := context.Background()
	repo := newWidgetTypeRepo(t)
	mustStoreWidgetType(t, makeWidgetType(t, "gauge", nil, repo), repo)
	releases := repositories.NewLibraryReleaseRepositoryPostgres(testDB)
	first := mustRelease(t, repo.libraryID)

	updated, err := releases.UpdateNote(ctx, repo.libraryID, first.Number(), "with a note")
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}
	if updated.Note() != "with a note" || updated.Number() != first.Number() || *updated.CommitNumber() != *first.CommitNumber() {
		t.Errorf("UpdateNote: %+v", updated)
	}
	if events := updated.PendingEvents(); len(events) != 1 || events[0].Type() != event.EventTypeLibraryReleaseNoteUpdated {
		t.Errorf("UpdateNote events: %v", events)
	}
	if read, err := releases.FindByNumber(ctx, repo.libraryID, first.Number()); err != nil || read.Note() != "with a note" {
		t.Errorf("the note read back: %v, %v", read, err)
	}

	// A Release of the days before Commits: no commit, system author, legacy.
	second := id.NewID[library.Library]()
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO engineering.libraries (id, organization_id, name, version) SELECT $1, id, $2, 1 FROM organizations`,
		second.UUID(), "legacy-"+uuid.NewString()); err != nil {
		t.Fatalf("insert legacy library: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		`INSERT INTO engineering.library_releases (library_id, number) VALUES ($1, 1)`, second.UUID()); err != nil {
		t.Fatalf("insert legacy release: %v", err)
	}
	legacy, err := releases.FindLatest(ctx, second)
	if err != nil {
		t.Fatalf("FindLatest legacy: %v", err)
	}
	if legacy.CommitNumber() != nil || legacy.Origin() != library.OriginLegacy || legacy.Author().String() != "system" {
		t.Errorf("the legacy release: %+v", legacy)
	}
	if numbers, err := releases.ReleasedCommits(ctx, second); err != nil || len(numbers) != 0 {
		t.Errorf("a legacy release names no commit: %v, %v", numbers, err)
	}
}

// TestLibraryReleaseState_UnsavedAndUnpublished: the Draft is unsaved when its
// Version is above the one its last Commit fixed, and unpublished while the
// last Commit is newer than the one the latest Release published
// (task 72_5; ADR 0021).
func TestLibraryReleaseState_UnsavedAndUnpublished(t *testing.T) {
	cleanLibraries(t)
	ctx := context.Background()
	repo := newWidgetTypeRepo(t)
	releases := repositories.NewLibraryReleaseRepositoryPostgres(testDB)
	check := func(step string, wantLatest int, wantUnsaved, wantUnpublished bool) {
		t.Helper()
		latest, err := releases.FindLatest(ctx, repo.libraryID)
		if err != nil {
			t.Fatalf("%s: FindLatest: %v", step, err)
		}
		lastCommit, found, err := testDrafts().FindLastCommit(ctx, draft.LibraryScope(repo.libraryID))
		if err != nil {
			t.Fatalf("%s: FindLastCommit: %v", step, err)
		}
		lastCommitNumber, lastCommitDraftVersion := 0, 0
		if found {
			lastCommitNumber = lastCommit.Number()
			lastCommitDraftVersion = lastCommit.DraftVersion().Number()
		}
		got := library.ReleaseStateOf(latest, lastCommitNumber, libraryDraftVersionOf(t, repo.libraryID).Number(), lastCommitDraftVersion)
		if got.Latest.Int() != wantLatest || got.UnsavedChanges != wantUnsaved || got.UnpublishedChanges != wantUnpublished {
			t.Errorf("%s: %+v, want release %d, unsaved %t, unpublished %t",
				step, got, wantLatest, wantUnsaved, wantUnpublished)
		}
	}

	check("a new library", 0, true, false)
	port, _ := library.NewInputPortName("value")
	ports := []library.InputPort{library.NewInputPort(port, "the value", library.AnyTypeHint())}
	gauge := mustStoreWidgetType(t, makeWidgetType(t, "gauge", ports, repo), repo)
	check("a Draft never saved", 0, true, false)
	mustRelease(t, repo.libraryID)
	check("saved and released", 1, false, false)

	update := func(html string, ports []library.InputPort) {
		t.Helper()
		h, _ := library.NewHtmlTemplate(html)
		changed, err := gauge.Update(gauge.Name(), h, gauge.Script(), gauge.ScriptLanguage(), gauge.DefaultSize(), ports)
		if err != nil {
			t.Fatal(err)
		}
		gauge = mustUpdateLibraryWidgetType(t, repo.libraryID, changed)
	}
	update("<p></p>", ports)
	check("changed since the commit", 1, true, false)
	mustRelease(t, repo.libraryID)
	check("saved and released again", 2, false, false)

	extra := mustStoreWidgetType(t, makeWidgetType(t, "extra", nil, repo), repo)
	check("a WidgetType added", 2, true, false)
	mustUpdateLibrary(t, repo.libraryID, "Delete widget type extra", func(session *draft.Session) error {
		_, err := session.DeleteWidgetType(ctx, extra.ID())
		return err
	})
	// The Draft is back to the released content, but its Version moved: it is
	// not at the last Commit.
	check("the added one removed", 2, true, false)
	mustRelease(t, repo.libraryID)
	check("released after the removal", 3, false, false)
}
