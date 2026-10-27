package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
)

func TestLibraryService_ReleaseNumbersAndRefusesAnUnchangedDraft(t *testing.T) {
	ctx := context.Background()
	svc := newLibraryService()
	lib := mustCreateLibrary(t)
	if !lib.HasUnreleasedChanges || lib.LatestRelease != 0 {
		t.Errorf("a new library: %+v, want unreleased changes and no release", lib)
	}
	if _, err := svc.Release(ctx, lib.ID, lib.Version+1, appdto.ReleaseInput{}); !errors.Is(err, library.ErrLibraryConflict) {
		t.Errorf("a stale version: expected ErrLibraryConflict, got %v", err)
	}
	wt, err := newWidgetTypeService().CreateWidgetType(ctx, lib.ID, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}

	first, err := svc.Release(ctx, lib.ID, lib.Version, appdto.ReleaseInput{Note: "first"})
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if first.Number != 1 || first.Note != "first" || len(first.WidgetTypes) != 1 || first.WidgetTypes[0].ID != wt.ID {
		t.Errorf("release 1: %+v", first)
	}
	if _, err := svc.Release(ctx, lib.ID, lib.Version, appdto.ReleaseInput{}); !errors.Is(err, library.ErrNothingToRelease) {
		t.Errorf("an unchanged Draft: expected ErrNothingToRelease, got %v", err)
	}
	found, err := svc.FindLibraryByID(ctx, lib.ID)
	if err != nil || found.HasUnreleasedChanges || found.LatestRelease != 1 {
		t.Errorf("after the release: %+v, %v", found, err)
	}

	if _, err := newWidgetTypeService().DeleteWidgetTypeByID(ctx, lib.ID, wt.ID); err != nil {
		t.Fatal(err)
	}
	if found, _ := svc.FindLibraryByID(ctx, lib.ID); !found.HasUnreleasedChanges {
		t.Error("a removed WidgetType is a change not released")
	}
	second, err := svc.Release(ctx, lib.ID, lib.Version, appdto.ReleaseInput{})
	if err != nil || second.Number != 2 || len(second.WidgetTypes) != 0 {
		t.Fatalf("release 2: %+v, %v", second, err)
	}
	all, err := svc.FindReleases(ctx, lib.ID)
	if err != nil || len(all) != 2 || all[0].Number != 1 {
		t.Errorf("FindReleases: %+v, %v", all, err)
	}
	if got, err := svc.FindRelease(ctx, lib.ID, 1); err != nil || len(got.WidgetTypes) != 1 {
		t.Errorf("FindRelease 1: %+v, %v", got, err)
	}
	if _, err := svc.FindRelease(ctx, lib.ID, 3); !errors.Is(err, library.ErrReleaseNotFound) {
		t.Errorf("a missing release: expected ErrReleaseNotFound, got %v", err)
	}
}

func TestLibraryService_DeleteRefusedWhileALiveProjectPinsIt(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	wt := r.createType(t, "value")
	libraryID := r.libraries[wt.ID]

	if _, err := r.libSvc.DeleteLibraryByID(ctx, libraryID); !errors.Is(err, library.ErrLibraryPinned) {
		t.Fatalf("pinned: expected ErrLibraryPinned, got %v", err)
	}
	if _, err := r.pinSvc.DeleteLibraryPin(ctx, mainProject, libraryID); err != nil {
		t.Fatalf("DeleteLibraryPin: %v", err)
	}
	deleted, err := r.libSvc.DeleteLibraryByID(ctx, libraryID)
	if err != nil {
		t.Fatalf("DeleteLibraryByID: %v", err)
	}
	if deleted.LatestRelease != 1 || deleted.HasUnreleasedChanges {
		t.Errorf("the deleted library: %+v, want it as it was, at release 1", deleted)
	}
	if _, err := r.libSvc.FindLibraryByID(ctx, libraryID); !errors.Is(err, library.ErrLibraryNotFound) {
		t.Errorf("a deleted library: expected ErrLibraryNotFound, got %v", err)
	}
	r.settle(t)

	var rows int
	if err := testDB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM engineering.libraries WHERE id = $1)
	          + (SELECT count(*) FROM engineering.widget_types WHERE library_id = $1)
	          + (SELECT count(*) FROM engineering.library_releases WHERE library_id = $1)`, libraryID).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("rows of the removed library: %d, %v", rows, err)
	}
	if jobs := jobsOf(t, application.LibraryCleanupPolicyName); len(jobs) != 0 {
		t.Errorf("jobs left: %+v", jobs)
	}
}

// TestDiscard_ResetsTheDraftToTheLatestRelease: the WidgetTypes added since
// go, the changed ones are restored, the removed ones come back under their
// IDs — two names swapped since the release included —; the Draft is
// read-only until the policy has run.
func TestDiscard_ResetsTheDraftToTheLatestRelease(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	lib := mustCreateLibrary(t)
	if _, err := r.libSvc.Discard(ctx, lib.ID); !errors.Is(err, library.ErrNoRelease) {
		t.Errorf("a library never released: expected ErrNoRelease, got %v", err)
	}
	create := func(name string) appdto.WidgetType {
		t.Helper()
		input := testCreateWidgetTypeInput
		input.Name = name
		wt, err := r.wtSvc.CreateWidgetType(ctx, lib.ID, input)
		if err != nil {
			t.Fatalf("CreateWidgetType %s: %v", name, err)
		}
		return wt
	}
	rename := func(wt appdto.WidgetType, name string) appdto.WidgetType {
		t.Helper()
		input := testCreateWidgetTypeInput
		input.Name = name
		updated, err := r.wtSvc.UpdateWidgetType(ctx, lib.ID, wt.ID, wt.Version, input)
		if err != nil {
			t.Fatalf("UpdateWidgetType %s: %v", name, err)
		}
		return updated
	}
	a, b, removed := create("a"), create("b"), create("removed")
	mustReleaseLibrary(t, lib.ID)

	if _, err := r.wtSvc.DeleteWidgetTypeByID(ctx, lib.ID, removed.ID); err != nil {
		t.Fatal(err)
	}
	a = rename(a, "tmp")
	b = rename(b, "a")
	rename(a, "b")
	create("added")

	marked, err := r.libSvc.Discard(ctx, lib.ID)
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if marked.DiscardingTo != 1 {
		t.Errorf("the mark: %+v", marked)
	}
	if _, err := r.libSvc.Discard(ctx, lib.ID); !errors.Is(err, library.ErrLibraryDiscarding) {
		t.Errorf("a second Discard: expected ErrLibraryDiscarding, got %v", err)
	}
	if _, err := r.wtSvc.CreateWidgetType(ctx, lib.ID, testCreateWidgetTypeInput); !errors.Is(err, library.ErrLibraryDiscarding) {
		t.Errorf("an edit during the Discard: expected ErrLibraryDiscarding, got %v", err)
	}
	if _, err := r.libSvc.Release(ctx, lib.ID, marked.Version, appdto.ReleaseInput{}); !errors.Is(err, library.ErrLibraryDiscarding) {
		t.Errorf("a Release during the Discard: expected ErrLibraryDiscarding, got %v", err)
	}

	r.settle(t)

	after, err := r.libSvc.FindLibraryByID(ctx, lib.ID)
	if err != nil || after.DiscardingTo != 0 || after.HasUnreleasedChanges {
		t.Errorf("after the Discard: %+v, %v", after, err)
	}
	draft, err := r.wtSvc.FindAllWidgetTypes(ctx, lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[uuid.UUID]string{}
	for _, wt := range draft {
		byID[wt.ID] = wt.Name
	}
	want := map[uuid.UUID]string{a.ID: "a", b.ID: "b", removed.ID: "removed"}
	if len(byID) != len(want) {
		t.Errorf("the Draft: %v, want %v", byID, want)
	}
	for id, name := range want {
		if byID[id] != name {
			t.Errorf("type %s: named %q, want %q", id, byID[id], name)
		}
	}
	if jobs := jobsOf(t, application.DiscardLibraryPolicyName); len(jobs) != 0 {
		t.Errorf("jobs left: %+v", jobs)
	}
	if _, err := r.wtSvc.CreateWidgetType(ctx, lib.ID, testCreateWidgetTypeInput); err != nil {
		t.Errorf("an edit after the Discard: %v", err)
	}
}

// TestLibraryPin_SwitchOrphaningWidgetsIsRefused_PreviewNamesThem: a switch
// or removal that would leave Widgets without their WidgetType is refused,
// naming them, and the preview names them and the bindings it would break.
func TestLibraryPin_SwitchOrphaningWidgetsIsRefused_PreviewNamesThem(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	kept := r.createType(t, "value")
	libraryID := r.libraries[kept.ID]
	dropped, err := r.wtSvc.CreateWidgetType(ctx, libraryID, appdto.WidgetTypeInput{
		Name: "dropped", HtmlTemplate: "<div></div>", ScriptLanguage: "javascript", DefaultWidth: 10, DefaultHeight: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	two := mustReleaseLibrary(t, libraryID)
	r.switchPin(t, kept.ID, two.Number)
	sc := r.createScene(t, "scene")
	onKept := r.addWidget(t, sc.ID, sc.Version, kept, "value")
	orphan := r.addWidget(t, sc.ID, onKept.SceneVersion, dropped)

	if _, err := r.wtSvc.DeleteWidgetTypeByID(ctx, libraryID, dropped.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.wtSvc.UpdateWidgetType(ctx, libraryID, kept.ID, kept.Version, appdto.WidgetTypeInput{
		Name: kept.Name, HtmlTemplate: kept.HtmlTemplate, ScriptLanguage: kept.ScriptLanguage,
		DefaultWidth: kept.DefaultWidth, DefaultHeight: kept.DefaultHeight,
	}); err != nil {
		t.Fatal(err)
	}
	three := mustReleaseLibrary(t, libraryID)

	impact, err := r.pinSvc.PinImpact(ctx, mainProject, libraryID, &three.Number)
	if err != nil {
		t.Fatalf("PinImpact: %v", err)
	}
	if len(impact.Orphaned) != 1 || impact.Orphaned[0].WidgetID != orphan.ID || impact.Orphaned[0].SceneID != sc.ID {
		t.Errorf("orphaned: %+v, want the widget of the dropped type", impact.Orphaned)
	}
	if len(impact.Broken) != 1 || impact.Broken[0].WidgetID != onKept.ID || impact.Broken[0].PortName != "value" {
		t.Errorf("broken: %+v, want the binding of the dropped port", impact.Broken)
	}
	removal, err := r.pinSvc.PinImpact(ctx, mainProject, libraryID, nil)
	if err != nil || len(removal.Orphaned) != 2 {
		t.Errorf("removal: %+v, %v; want both widgets orphaned", removal, err)
	}
	unpinned := mustCreateLibrary(t)
	if _, err := r.pinSvc.PinImpact(ctx, mainProject, unpinned.ID, nil); !errors.Is(err, librarypin.ErrLibraryPinNotFound) {
		t.Errorf("removal of a pin the project does not have: expected ErrLibraryPinNotFound, got %v", err)
	}

	pins, _ := r.pinSvc.FindLibraryPins(ctx, mainProject)
	i := slices.IndexFunc(pins, func(p appdto.LibraryPin) bool { return p.LibraryID == libraryID })
	if i < 0 {
		t.Fatal("the library is not pinned")
	}
	pin := pins[i]
	_, _, err = r.pinSvc.PutLibraryPin(ctx, mainProject, libraryID, three.Number, pin.Version)
	var orphans librarypin.OrphansError
	if !errors.As(err, &orphans) || len(orphans.Widgets) != 1 || orphans.Widgets[0].Widget.ID().UUID() != orphan.ID {
		t.Errorf("the switch: expected an OrphansError naming the widget, got %v", err)
	}
	if _, err := r.pinSvc.DeleteLibraryPin(ctx, mainProject, libraryID); !errors.Is(err, librarypin.ErrWidgetsOrphaned) {
		t.Errorf("the removal: expected ErrWidgetsOrphaned, got %v", err)
	}
	if _, _, err := r.pinSvc.PutLibraryPin(ctx, mainProject, libraryID, three.Number, pin.Version-1); !errors.Is(err, librarypin.ErrLibraryPinConflict) {
		t.Errorf("a stale switch: expected ErrLibraryPinConflict, got %v", err)
	}
	if _, _, err := r.pinSvc.PutLibraryPin(ctx, mainProject, libraryID, 9, pin.Version); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("a missing release: expected ErrInvalidInput, got %v", err)
	}

	// Once the orphan is gone, the switch passes and the policy removes the
	// broken binding.
	if _, err := r.sceneSvc.DeleteWidgetByID(ctx, mainProject, sc.ID, orphan.ID); err != nil {
		t.Fatal(err)
	}
	switched, created, err := r.pinSvc.PutLibraryPin(ctx, mainProject, libraryID, three.Number, pin.Version)
	if err != nil || created || switched.Release != three.Number {
		t.Fatalf("switch: %+v, %t, %v", switched, created, err)
	}
	r.settle(t)
	if got := r.boundPorts(t, sc.ID, onKept.ID); len(got) != 0 {
		t.Errorf("bound to %v after the switch, want none", got)
	}
	types, err := r.pinSvc.FindPinnedWidgetTypes(ctx, mainProject)
	if err != nil || slices.ContainsFunc(types, func(wt appdto.ReleasedWidgetType) bool { return wt.ID == dropped.ID }) {
		t.Errorf("pinned types: %+v, %v; want no dropped type", types, err)
	}
}

func TestLibraryPin_AddRequiresAReleaseAndPinsOnce(t *testing.T) {
	ctx := context.Background()
	svc := newLibraryPinService()
	lib := mustCreateLibrary(t)
	if _, _, err := svc.PutLibraryPin(ctx, mainProject, lib.ID, 1, 0); !errors.Is(err, application.ErrInvalidInput) || !errors.Is(err, library.ErrReleaseNotFound) {
		t.Errorf("a library never released: expected invalid input of a missing release, got %v", err)
	}
	mustReleaseLibrary(t, lib.ID)
	pin, created, err := svc.PutLibraryPin(ctx, mainProject, lib.ID, 1, 0)
	if err != nil || !created || pin.Release != 1 || pin.Version != 1 {
		t.Fatalf("add: %+v, %t, %v", pin, created, err)
	}
	if _, _, err := svc.PutLibraryPin(ctx, mainProject, lib.ID, 1, 0); !errors.Is(err, librarypin.ErrLibraryPinConflict) {
		t.Errorf("adding again: expected ErrLibraryPinConflict, got %v", err)
	}
	if _, _, err := svc.PutLibraryPin(ctx, mainProject, uuid.New(), 1, 0); !errors.Is(err, library.ErrLibraryNotFound) {
		t.Errorf("a missing library: expected ErrLibraryNotFound, got %v", err)
	}
	if _, err := svc.DeleteLibraryPin(ctx, mainProject, lib.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := svc.DeleteLibraryPin(ctx, mainProject, lib.ID); !errors.Is(err, librarypin.ErrLibraryPinNotFound) {
		t.Errorf("removing again: expected ErrLibraryPinNotFound, got %v", err)
	}
}
