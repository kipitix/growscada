package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/repositories"
)

func TestLibrarySave_RenameNameTakenAndDiscardMark(t *testing.T) {
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

	// A Discard to a release no longer the latest is a conflict.
	first := mustRelease(t, lib.ID())
	wts := libraryRepo{repositories.NewWidgetTypeRepositoryPostgres(testDB, lib.ID()), lib.ID()}
	if _, err := wts.Save(ctx, makeWidgetType(t, "gauge", nil, wts)); err != nil {
		t.Fatal(err)
	}
	two := mustRelease(t, lib.ID()).Number()
	found, _ := repo.FindByID(ctx, lib.ID())
	stale, _ := found.StartDiscard(first.Number())
	if _, err := repo.Save(ctx, stale); !errors.Is(err, library.ErrLibraryConflict) {
		t.Errorf("a Discard to release 1 of 2: expected ErrLibraryConflict, got %v", err)
	}
	marked, _ := found.StartDiscard(two)
	saved, err := repo.Save(ctx, marked)
	if err != nil {
		t.Fatalf("Save marked: %v", err)
	}
	reread, _ := repo.FindByID(ctx, lib.ID())
	if to, ok := reread.DiscardingTo(); !ok || to != two || reread.Version() != saved.Version() {
		t.Errorf("the mark: %s", reread)
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
	saved, err := repo.Save(ctx, makeWidgetType(t, "gauge", []library.InputPort{library.NewInputPort(port, "the value", library.AnyTypeHint())}, repo))
	if err != nil {
		t.Fatalf("Save widget type: %v", err)
	}
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
	if _, err := releases.Save(ctx, again, lib.Version()); !errors.Is(err, library.ErrReleaseNumberTaken) {
		t.Errorf("number 1 again: expected ErrReleaseNumberTaken, got %v", err)
	}
	second, _ := library.CreateRelease(lib, nil, first, "emptied", time.Now())
	if _, err := releases.Save(ctx, second, lib.Version()); err != nil {
		t.Fatalf("Save second: %v", err)
	}
	all, err := releases.FindAll(ctx, repo.libraryID)
	if err != nil || len(all) != 2 || all[1].Note() != "emptied" || len(all[1].WidgetTypes()) != 0 || len(all[0].WidgetTypes()) != 1 {
		t.Errorf("FindAll: %v, %v", all, err)
	}
	if latest, _ := releases.FindLatest(ctx, repo.libraryID); latest.Number().Int() != 2 {
		t.Errorf("FindLatest: %s", latest)
	}

	marked, _ := lib.StartDiscard(first.Number().Next())
	markedSaved, err := repositories.NewLibraryRepositoryPostgres(testDB).Save(ctx, marked)
	if err != nil {
		t.Fatal(err)
	}
	third, _ := library.CreateRelease(lib, nil, nil, "", time.Now())
	third = library.WithNumber(third, library.FirstRelease().Next().Next())
	if _, err := releases.Save(ctx, third, lib.Version()); !errors.Is(err, library.ErrLibraryConflict) {
		t.Errorf("a release of a library changed since it was read: expected ErrLibraryConflict, got %v", err)
	}
	if _, err := releases.Save(ctx, third, markedSaved.Version()); !errors.Is(err, library.ErrLibraryDiscarding) {
		t.Errorf("a release during a Discard: expected ErrLibraryDiscarding, got %v", err)
	}
}

func TestLibraryPin_AddSwitchRemove(t *testing.T) {
	cleanScenes(t)
	cleanLibraries(t)
	ctx := context.Background()
	repo := newWidgetTypeRepo(t)
	saved, err := repo.Save(ctx, makeWidgetType(t, "gauge", nil, repo))
	if err != nil {
		t.Fatal(err)
	}
	first := mustRelease(t, repo.libraryID)
	pins := repositories.NewLibraryPinRepositoryPostgres(testDB, mainProject)

	missing := library.ReconstituteRelease(repo.libraryID, first.Number().Next(), "", time.Now(), nil)
	if _, err := pins.Save(ctx, librarypin.CreateLibraryPin(mainProject, missing), nil); !errors.Is(err, library.ErrReleaseNotFound) {
		t.Errorf("a missing release: expected ErrReleaseNotFound, got %v", err)
	}
	pin, err := pins.Save(ctx, librarypin.CreateLibraryPin(mainProject, first), nil)
	if err != nil {
		t.Fatalf("Save pin: %v", err)
	}
	if _, err := pins.Save(ctx, librarypin.CreateLibraryPin(mainProject, first), nil); !errors.Is(err, librarypin.ErrLibraryPinExists) {
		t.Errorf("pinned twice: expected ErrLibraryPinExists, got %v", err)
	}
	types, err := pins.FindPinnedWidgetTypes(ctx)
	if err != nil || len(types) != 1 || types[0].ID() != saved.ID() {
		t.Errorf("FindPinnedWidgetTypes: %v, %v", types, err)
	}
	unpinned := repo.NextID()
	if types, err := pins.FindPinnedWidgetTypesByID(ctx, []id.ID[library.WidgetType]{unpinned, saved.ID()}); err != nil || len(types) != 1 || types[0].ID() != saved.ID() {
		t.Errorf("FindPinnedWidgetTypesByID: %v, %v; want the pinned one only", types, err)
	}
	if types, err := pins.FindPinnedWidgetTypesByID(ctx, []id.ID[library.WidgetType]{unpinned}); err != nil || len(types) != 0 {
		t.Errorf("FindPinnedWidgetTypesByID of an unpinned type: %v, %v", types, err)
	}

	// A widget of the pinned type; the switch's check sees it.
	sceneRepo := repositories.NewSceneRepositoryPostgres(testDB, mainProject)
	name, _ := scene.NewWidgetName("w")
	w := scene.NewWidget(sceneRepo.NextWidgetID(), name, scene.NewPosition(0, 0, 0), library.DefaultSize(), scene.DefaultOrigin(), scene.DefaultRotation(), saved.ID(), nil, nil)
	sc := mustSave(t, sceneRepo, withWidgets(makeScene(t, sceneRepo, "s"), w))

	if err := repo.Delete(ctx, saved.Delete()); err != nil {
		t.Fatal(err)
	}
	second := mustRelease(t, repo.libraryID)
	switched, _ := pin.Switch(pin.Version(), second)
	var seen []librarypin.PlacedWidget
	refuse := errors.New("refused")
	if _, err := pins.Save(ctx, switched, func(ws []librarypin.PlacedWidget) error { seen = ws; return refuse }); !errors.Is(err, refuse) {
		t.Errorf("the check's error: got %v", err)
	}
	if len(seen) != 1 || seen[0].Widget.ID() != w.ID() || seen[0].SceneID != sc.ID() {
		t.Errorf("the check saw %v", seen)
	}
	if got, _ := pins.FindByLibrary(ctx, repo.libraryID); got.Release() != first.Number() {
		t.Errorf("a refused switch changed the pin: %s", got)
	}
	if _, err := pins.Save(ctx, switched, nil); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if _, err := pins.Save(ctx, switched, nil); !errors.Is(err, librarypin.ErrLibraryPinConflict) {
		t.Errorf("a stale switch: expected ErrLibraryPinConflict, got %v", err)
	}
	// A removal checked against the release pinned before the switch is stale.
	if err := pins.Delete(ctx, switched.Delete(), nil); !errors.Is(err, librarypin.ErrLibraryPinConflict) {
		t.Errorf("a stale removal: expected ErrLibraryPinConflict, got %v", err)
	}
	current, err := pins.FindByLibrary(ctx, repo.libraryID)
	if err != nil {
		t.Fatal(err)
	}
	if err := pins.Delete(ctx, current.Delete(), func([]librarypin.PlacedWidget) error { return refuse }); !errors.Is(err, refuse) {
		t.Errorf("a refused removal: got %v", err)
	}
	if err := pins.Delete(ctx, current.Delete(), nil); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := pins.FindByLibrary(ctx, repo.libraryID); !errors.Is(err, librarypin.ErrLibraryPinNotFound) {
		t.Errorf("after removal: expected ErrLibraryPinNotFound, got %v", err)
	}

	gone := mustSaveLibrary(t)
	libraries := repositories.NewLibraryRepositoryPostgres(testDB)
	goneRelease := mustRelease(t, gone.ID())
	if _, err := libraries.Save(ctx, gone.Delete()); err != nil {
		t.Fatal(err)
	}
	if _, err := pins.Save(ctx, librarypin.CreateLibraryPin(mainProject, goneRelease), nil); !errors.Is(err, library.ErrLibraryNotFound) {
		t.Errorf("a deleted library: expected ErrLibraryNotFound, got %v", err)
	}
	otherProject := id.NewID[project.Project]()
	if _, err := pins.Save(ctx, librarypin.CreateLibraryPin(otherProject, first), nil); err == nil {
		t.Error("a pin of another project through Main's repository must fail")
	}
}

// TestLibraryReleaseState_OfTheStoredDraftAndRelease: a Draft and its latest
// release read back from the database differ exactly when a WidgetType was
// changed, added or removed since the release, whatever was stored.
func TestLibraryReleaseState_OfTheStoredDraftAndRelease(t *testing.T) {
	cleanLibraries(t)
	ctx := context.Background()
	repo := newWidgetTypeRepo(t)
	releases := repositories.NewLibraryReleaseRepositoryPostgres(testDB)
	check := func(step string, wantLatest int, wantDiffers bool) {
		t.Helper()
		draft, err := repo.FindAll(ctx)
		if err != nil {
			t.Fatalf("%s: FindAll: %v", step, err)
		}
		latest, err := releases.FindLatest(ctx, repo.libraryID)
		if err != nil {
			t.Fatalf("%s: FindLatest: %v", step, err)
		}
		if got := library.ReleaseStateOf(draft, latest); got.Latest.Int() != wantLatest || got.DraftDiffers != wantDiffers {
			t.Errorf("%s: %+v, want release %d, differs %t", step, got, wantLatest, wantDiffers)
		}
	}

	check("a new library", 0, true)
	port, _ := library.NewInputPortName("value")
	ports := []library.InputPort{library.NewInputPort(port, "the value", library.AnyTypeHint())}
	gauge, err := repo.Save(ctx, makeWidgetType(t, "gauge", ports, repo))
	if err != nil {
		t.Fatal(err)
	}
	mustRelease(t, repo.libraryID)
	check("released", 1, false)

	update := func(step string, html string, ports []library.InputPort) {
		t.Helper()
		h, _ := library.NewHtmlTemplate(html)
		changed, err := gauge.Update(gauge.Version(), gauge.Name(), h, gauge.Script(), gauge.ScriptLanguage(), gauge.DefaultSize(), ports)
		if err != nil {
			t.Fatal(err)
		}
		if gauge, err = repo.Save(ctx, changed); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
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

	extra, err := repo.Save(ctx, makeWidgetType(t, "extra", nil, repo))
	if err != nil {
		t.Fatal(err)
	}
	check("a WidgetType added", 2, true)
	if err := repo.Delete(ctx, extra.Delete()); err != nil {
		t.Fatal(err)
	}
	check("the added one removed", 2, false)
	if err := repo.Delete(ctx, gauge.Delete()); err != nil {
		t.Fatal(err)
	}
	check("a released WidgetType removed", 2, true)
}
