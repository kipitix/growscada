package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func TestLibrarySave_RenameAndNameTaken(t *testing.T) {
	cleanLibraries(t)
	ctx := editlock.System(context.Background())
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
	ctx := editlock.System(context.Background())
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
	ctx := editlock.System(context.Background())
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

	lib, _ := repositories.NewLibraryRepositoryPostgres(testDB).FindByID(ctx, repo.libraryID)
	again, _ := library.CreateRelease(lib, []library.WidgetType{}, nil, "", time.Now())
	if _, err := releases.Save(ctx, again, libraryDraftContentVersionOf(t, repo.libraryID)); !errors.Is(err, library.ErrReleaseNumberTaken) {
		t.Errorf("number 1 again: expected ErrReleaseNumberTaken, got %v", err)
	}
	if _, err := testDrafts().Update(ctx, editlock.LibraryScope(repo.libraryID), libraryDraftVersionOf(t, repo.libraryID),
		"Delete widget type gauge", func(session *draft.Session) error {
			_, err := session.DeleteWidgetType(ctx, saved.ID())
			return err
		}); err != nil {
		t.Fatalf("Delete widget type: %v", err)
	}
	second, _ := library.CreateRelease(lib, nil, first, "emptied", time.Now())
	if _, err := releases.Save(ctx, second, libraryDraftContentVersionOf(t, repo.libraryID)); err != nil {
		t.Fatalf("Save second: %v", err)
	}
	all, err := releases.FindAll(ctx, repo.libraryID)
	if err != nil || len(all) != 2 || all[1].Note() != "emptied" || len(all[1].WidgetTypes()) != 0 || len(all[0].WidgetTypes()) != 1 {
		t.Errorf("FindAll: %v, %v", all, err)
	}
	if latest, _ := releases.FindLatest(ctx, repo.libraryID); latest.Number().Int() != 2 {
		t.Errorf("FindLatest: %s", latest)
	}
}

// TestLibraryRelease_SaveRefusesAStaleDraftVersion: the Release states the
// Version of the Draft the client read; a write since then is a conflict and
// stores nothing (ADR 0016).
func TestLibraryRelease_SaveRefusesAStaleDraftVersion(t *testing.T) {
	cleanLibraries(t)
	ctx := editlock.System(context.Background())
	repo := newWidgetTypeRepo(t)
	mustStoreWidgetType(t, makeWidgetType(t, "gauge", nil, repo), repo)
	releases := repositories.NewLibraryReleaseRepositoryPostgres(testDB)
	lib, err := repositories.NewLibraryRepositoryPostgres(testDB).FindByID(ctx, repo.libraryID)
	if err != nil {
		t.Fatalf("FindByID library: %v", err)
	}
	read, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll widget types: %v", err)
	}
	readVersion := libraryDraftContentVersionOf(t, repo.libraryID)
	stale, err := library.CreateRelease(lib, read, nil, "", time.Now())
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}

	mustStoreWidgetType(t, makeWidgetType(t, "dial", nil, repo), repo)
	if _, err := releases.Save(ctx, stale, readVersion); !errors.Is(err, draft.ErrDraftConflict) {
		t.Fatalf("a release at a stale Draft Version: expected ErrDraftConflict, got %v", err)
	}
	if latest, err := releases.FindLatest(ctx, repo.libraryID); err != nil || latest != nil {
		t.Errorf("the refused release was stored: %v, %v", latest, err)
	}

	draftNow, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll widget types: %v", err)
	}
	fresh, err := library.CreateRelease(lib, draftNow, nil, "", time.Now())
	if err != nil {
		t.Fatalf("CreateRelease of the Draft as it is: %v", err)
	}
	if _, err := releases.Save(ctx, fresh, libraryDraftContentVersionOf(t, repo.libraryID)); err != nil {
		t.Fatalf("Save release of the Draft as it is: %v", err)
	}
}

// TestLibraryReleaseState_OfTheStoredDraftAndRelease: a Draft and its latest
// release read back from the database differ exactly when a WidgetType was
// changed, added or removed since the release, whatever was stored.
func TestLibraryReleaseState_OfTheStoredDraftAndRelease(t *testing.T) {
	cleanLibraries(t)
	ctx := editlock.System(context.Background())
	repo := newWidgetTypeRepo(t)
	releases := repositories.NewLibraryReleaseRepositoryPostgres(testDB)
	check := func(step string, wantLatest int, wantDiffers bool) {
		t.Helper()
		draftNow, err := repo.FindAll(ctx)
		if err != nil {
			t.Fatalf("%s: FindAll: %v", step, err)
		}
		latest, err := releases.FindLatest(ctx, repo.libraryID)
		if err != nil {
			t.Fatalf("%s: FindLatest: %v", step, err)
		}
		if got := library.ReleaseStateOf(draftNow, latest); got.Latest.Int() != wantLatest || got.DraftDiffers != wantDiffers {
			t.Errorf("%s: %+v, want release %d, differs %t", step, got, wantLatest, wantDiffers)
		}
	}

	check("a new library", 0, true)
	port, _ := library.NewInputPortName("value")
	ports := []library.InputPort{library.NewInputPort(port, "the value", library.AnyTypeHint())}
	gauge := mustStoreWidgetType(t, makeWidgetType(t, "gauge", ports, repo), repo)
	mustRelease(t, repo.libraryID)
	check("released", 1, false)

	update := func(step string, html string, ports []library.InputPort) {
		t.Helper()
		h, _ := library.NewHtmlTemplate(html)
		changed, err := gauge.Update(gauge.Name(), h, gauge.Script(), gauge.ScriptLanguage(), gauge.DefaultSize(), ports)
		if err != nil {
			t.Fatal(err)
		}
		gauge = mustUpdateLibraryWidgetType(t, repo.libraryID, changed)
	}
	update("html", "<p></p>", ports)
	check("html changed", 1, true)
	h, _ := library.NewHtmlTemplate("<div></div>")
	update("html restored", h.String(), ports)
	check("html restored", 1, false)
	update("ports", h.String(), nil)
	check("ports changed", 1, true)
	mustRelease(t, repo.libraryID)
	check("released again", 2, false)

	extra := mustStoreWidgetType(t, makeWidgetType(t, "extra", nil, repo), repo)
	check("a WidgetType added", 2, true)
	mustUpdateLibrary(t, repo.libraryID, "Delete widget type extra", func(session *draft.Session) error {
		_, err := session.DeleteWidgetType(ctx, extra.ID())
		return err
	})
	check("the added one removed", 2, false)
	mustUpdateLibrary(t, repo.libraryID, "Delete widget type gauge", func(session *draft.Session) error {
		_, err := session.DeleteWidgetType(ctx, gauge.ID())
		return err
	})
	check("a released WidgetType removed", 2, true)
}
