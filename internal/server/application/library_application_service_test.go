package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

func TestLibraryService_ReleasePublishesACommit(t *testing.T) {
	ctx := context.Background()
	svc := newLibraryService()
	lib := mustCreateLibrary(t)
	if !lib.HasUnsavedChanges || lib.HasUnpublishedChanges || lib.LatestRelease != 0 || lib.LatestCommit != 0 {
		t.Errorf("a new library: %+v, want unsaved changes and no commit or release", lib)
	}
	if _, err := svc.Release(ctx, lib.ID, appdto.ReleaseInput{Commit: 7, Author: "Ada"}); !errors.Is(err, draft.ErrCommitNotFound) {
		t.Errorf("a commit the library does not have: expected ErrCommitNotFound, got %v", err)
	}
	wt, err := newWidgetTypeService().CreateWidgetType(ctx, lib.ID, libraryDraftVersionOf(t, lib.ID), testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	commit := mustCommitLibrary(t, lib.ID)
	if found, _ := svc.FindLibraryByID(ctx, lib.ID); found.HasUnsavedChanges || !found.HasUnpublishedChanges || found.LatestCommit != commit.Number {
		t.Errorf("after the Commit: %+v, want unsaved changes gone and the commit unpublished", found)
	}

	if _, err := svc.Release(ctx, lib.ID, appdto.ReleaseInput{Commit: commit.Number}); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("a release without an author: expected invalid input, got %v", err)
	}
	first, err := svc.Release(ctx, lib.ID, appdto.ReleaseInput{Commit: commit.Number, Note: "first", Author: "Ada"})
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if first.Number != 1 || first.Note != "first" || first.CommitNumber == nil || *first.CommitNumber != commit.Number ||
		first.Title != commit.Title || first.Author != "Ada" || first.Origin != "commit" ||
		len(first.WidgetTypes) != 1 || first.WidgetTypes[0].ID != wt.ID {
		t.Errorf("release 1: %+v", first)
	}
	if _, err := svc.Release(ctx, lib.ID, appdto.ReleaseInput{Commit: commit.Number, Author: "Ada"}); !errors.Is(err, library.ErrCommitAlreadyReleased) {
		t.Errorf("a released commit: expected ErrCommitAlreadyReleased, got %v", err)
	}
	found, err := svc.FindLibraryByID(ctx, lib.ID)
	if err != nil || found.HasUnsavedChanges || found.HasUnpublishedChanges || found.LatestRelease != 1 {
		t.Errorf("after the release: %+v, %v", found, err)
	}

	if _, err := newWidgetTypeService().DeleteWidgetTypeByID(ctx, lib.ID, wt.ID, libraryDraftVersionOf(t, lib.ID)); err != nil {
		t.Fatal(err)
	}
	if found, _ := svc.FindLibraryByID(ctx, lib.ID); !found.HasUnsavedChanges {
		t.Error("a removed WidgetType is a change not saved")
	}
	secondCommit := mustCommitLibrary(t, lib.ID)
	second, err := svc.Release(ctx, lib.ID, appdto.ReleaseInput{Commit: secondCommit.Number, Author: "Ada"})
	if err != nil || second.Number != 2 || len(second.WidgetTypes) != 0 {
		t.Fatalf("release 2: %+v, %v", second, err)
	}
	all, err := svc.FindReleases(ctx, lib.ID)
	if err != nil || len(all) != 2 || all[0].Number != 1 || all[1].CommitNumber == nil || *all[1].CommitNumber != secondCommit.Number {
		t.Errorf("FindReleases: %+v, %v", all, err)
	}
	if got, err := svc.FindRelease(ctx, lib.ID, 1); err != nil || len(got.WidgetTypes) != 1 || got.Title != commit.Title {
		t.Errorf("FindRelease 1: %+v, %v", got, err)
	}
	if _, err := svc.FindRelease(ctx, lib.ID, 3); !errors.Is(err, library.ErrReleaseNotFound) {
		t.Errorf("a missing release: expected ErrReleaseNotFound, got %v", err)
	}
	if updated, err := svc.UpdateReleaseNote(ctx, lib.ID, 1, "renoted"); err != nil || updated.Note != "renoted" || len(updated.WidgetTypes) != 1 {
		t.Errorf("UpdateReleaseNote: %+v, %v", updated, err)
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
	if _, err := r.pinSvc.DeleteLibraryPin(ctx, mainProject, libraryID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteLibraryPin: %v", err)
	}
	deleted, err := r.libSvc.DeleteLibraryByID(ctx, libraryID)
	if err != nil {
		t.Fatalf("DeleteLibraryByID: %v", err)
	}
	if deleted.LatestRelease != 1 {
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

// TestLibraryService_DeleteRefusedWhileARunningDeploymentNamesIt: a Release in
// the running Deployment of a Project keeps the Library, even with no pin; a
// Deployment, a Commit and a Project Release of the past do not (task 71).
func TestLibraryService_DeleteRefusedWhileARunningDeploymentNamesIt(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	wt := r.createType(t, "value")
	libraryID := r.libraries[wt.ID]
	deployProject(t, mainProject) // the running Deployment names the release

	// The pin goes; the Deployment keeps naming the library.
	if _, err := r.pinSvc.DeleteLibraryPin(ctx, mainProject, libraryID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("DeleteLibraryPin: %v", err)
	}
	if _, err := r.libSvc.DeleteLibraryByID(ctx, libraryID); !errors.Is(err, application.ErrLibraryInRunningDeployment) {
		t.Fatalf("named by the running Deployment: expected ErrLibraryInRunningDeployment, got %v", err)
	}

	// A new Deployment without the pin leaves the library in history alone: a
	// Commit or an old Deployment does not keep it. A Project Release of that
	// Commit copies the pins but does not keep it either.
	deployProject(t, mainProject)
	saved, err := newCommitService().Save(ctx,
		draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](mainProject))),
		draftVersionOf(t, mainProject), "With the library", "Ada")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// The test needs a Project Release of this Commit; a release left by
	// another test of the same Project names another Commit.
	if _, err := testDB.ExecContext(ctx, "DELETE FROM engineering.project_releases WHERE project_id = $1", mainProject); err != nil {
		t.Fatalf("clean releases: %v", err)
	}
	if _, err := newReleaseService().Create(ctx, mainProject, saved.Number, "", "Ada"); err != nil {
		t.Fatalf("Create release: %v", err)
	}
	if _, err := r.libSvc.DeleteLibraryByID(ctx, libraryID); err != nil {
		t.Fatalf("named by an old Deployment, a Commit and a Release only: %v", err)
	}
}

// TestDiscard_ResetsTheDraftToTheLastCommit: one transaction brings the
// WidgetTypes added since the Commit go, the changed ones restored and the
// removed ones back under their IDs — two names swapped since the Commit
// included.
func TestDiscard_ResetsTheDraftToTheLastCommit(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	lib := mustCreateLibrary(t)
	if _, err := r.libSvc.Discard(ctx, lib.ID); !errors.Is(err, library.ErrNothingToDiscard) {
		t.Errorf("a library never saved: expected ErrNothingToDiscard, got %v", err)
	}
	create := func(name string) appdto.WidgetType {
		t.Helper()
		input := testCreateWidgetTypeInput
		input.Name = name
		wt, err := r.wtSvc.CreateWidgetType(ctx, lib.ID, libraryDraftVersionOf(t, lib.ID), input)
		if err != nil {
			t.Fatalf("CreateWidgetType %s: %v", name, err)
		}
		return wt
	}
	rename := func(wt appdto.WidgetType, name string) appdto.WidgetType {
		t.Helper()
		input := testCreateWidgetTypeInput
		input.Name = name
		updated, err := r.wtSvc.UpdateWidgetType(ctx, lib.ID, wt.ID, libraryDraftVersionOf(t, lib.ID), input)
		if err != nil {
			t.Fatalf("UpdateWidgetType %s: %v", name, err)
		}
		return updated
	}
	a, b, removed := create("a"), create("b"), create("removed")
	mustReleaseLibrary(t, lib.ID)

	if _, err := r.wtSvc.DeleteWidgetTypeByID(ctx, lib.ID, removed.ID, libraryDraftVersionOf(t, lib.ID)); err != nil {
		t.Fatal(err)
	}
	a = rename(a, "tmp")
	b = rename(b, "a")
	rename(a, "b")
	create("added")

	state, err := r.libSvc.Discard(ctx, lib.ID)
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	after, err := r.libSvc.FindLibraryByID(ctx, lib.ID)
	if err != nil || after.HasUnpublishedChanges {
		t.Errorf("after the Discard: %+v, %v", after, err)
	}
	if state.Version <= 0 {
		t.Errorf("the Draft state after the Discard: %+v", state)
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
	// The Draft is editable again at once: one transaction, no mark.
	create("after-discard")
}

// TestLibraryPin_SwitchOrphaningWidgetsIsRefused_PreviewNamesThem: a switch
// or removal that would leave Widgets without their WidgetType is refused,
// naming them, and the preview names them and the bindings it would break.
func TestLibraryPin_SwitchOrphaningWidgetsIsRefused_PreviewNamesThem(t *testing.T) {
	r := newReconciliation(t, reconciliationWiring{})
	ctx := context.Background()
	kept := r.createType(t, "value")
	libraryID := r.libraries[kept.ID]
	dropped, err := r.wtSvc.CreateWidgetType(ctx, libraryID, libraryDraftVersionOf(t, libraryID), appdto.WidgetTypeInput{
		Name: "dropped", HtmlTemplate: "<div></div>", ScriptLanguage: "javascript", DefaultWidth: 10, DefaultHeight: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	two := mustReleaseLibrary(t, libraryID)
	r.switchPin(t, kept.ID, two.Number)
	sc := r.createScene(t, "scene")
	onKept := r.addWidget(t, sc.ID, sc.Version, kept, "value")
	orphan := r.addWidget(t, sc.ID, onKept.Version, dropped)

	if _, err := r.wtSvc.DeleteWidgetTypeByID(ctx, libraryID, dropped.ID, libraryDraftVersionOf(t, libraryID)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.wtSvc.UpdateWidgetType(ctx, libraryID, kept.ID, libraryDraftVersionOf(t, libraryID), appdto.WidgetTypeInput{
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
	_, _, err = r.pinSvc.PutLibraryPin(ctx, mainProject, libraryID, three.Number, draftVersionOf(t, mainProject))
	var orphans librarypin.OrphansError
	if !errors.As(err, &orphans) || len(orphans.Widgets) != 1 || orphans.Widgets[0].Widget.ID().UUID() != orphan.ID {
		t.Errorf("the switch: expected an OrphansError naming the widget, got %v", err)
	}
	if _, err := r.pinSvc.DeleteLibraryPin(ctx, mainProject, libraryID, draftVersionOf(t, mainProject)); !errors.Is(err, librarypin.ErrWidgetsOrphaned) {
		t.Errorf("the removal: expected ErrWidgetsOrphaned, got %v", err)
	}
	if _, _, err := r.pinSvc.PutLibraryPin(ctx, mainProject, libraryID, three.Number, draftVersionOf(t, mainProject)-1); !errors.Is(err, draft.ErrDraftConflict) {
		t.Errorf("a stale switch: expected draft.ErrDraftConflict, got %v", err)
	}
	if _, _, err := r.pinSvc.PutLibraryPin(ctx, mainProject, libraryID, 9, pin.Version); !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("a missing release: expected ErrInvalidInput, got %v", err)
	}

	// Once the orphan is gone, the switch passes and the policy removes the
	// broken binding.
	if _, err := r.sceneSvc.DeleteWidgetByID(ctx, mainProject, sc.ID, orphan.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatal(err)
	}
	switched, created, err := r.pinSvc.PutLibraryPin(ctx, mainProject, libraryID, three.Number, draftVersionOf(t, mainProject))
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
	if _, _, err := svc.PutLibraryPin(ctx, mainProject, lib.ID, 1, draftVersionOf(t, mainProject)); !errors.Is(err, application.ErrInvalidInput) || !errors.Is(err, library.ErrReleaseNotFound) {
		t.Errorf("a library never released: expected invalid input of a missing release, got %v", err)
	}
	mustReleaseLibrary(t, lib.ID)
	pin, created, err := svc.PutLibraryPin(ctx, mainProject, lib.ID, 1, draftVersionOf(t, mainProject))
	if err != nil || !created || pin.Release != 1 || pin.Version == 0 {
		t.Fatalf("add: %+v, %t, %v", pin, created, err)
	}
	if pin.CommitNumber == nil || pin.Title != "Save" || pin.Author != "Ada" || pin.Origin != "commit" {
		t.Errorf("the provenance of the added pin: %+v, want commit 1 titled %q by Ada", pin, "Save")
	}
	if again, added, err := svc.PutLibraryPin(ctx, mainProject, lib.ID, 1, draftVersionOf(t, mainProject)); err != nil || added || again.Release != 1 {
		t.Errorf("pinning the same release again: %+v, %t, %v; want the pin as it is", again, added, err)
	} else if again.Title != "Save" || again.CommitNumber == nil {
		t.Errorf("the provenance of the same pin: %+v", again)
	}
	if _, _, err := svc.PutLibraryPin(ctx, mainProject, uuid.New(), 1, draftVersionOf(t, mainProject)); !errors.Is(err, library.ErrLibraryNotFound) {
		t.Errorf("a missing library: expected ErrLibraryNotFound, got %v", err)
	}
	if _, err := svc.DeleteLibraryPin(ctx, mainProject, lib.ID, draftVersionOf(t, mainProject)); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := svc.DeleteLibraryPin(ctx, mainProject, lib.ID, draftVersionOf(t, mainProject)); !errors.Is(err, librarypin.ErrLibraryPinNotFound) {
		t.Errorf("removing again: expected ErrLibraryPinNotFound, got %v", err)
	}
}

// TestLibraryPin_ProvenanceOfEachLibrary: two Libraries, each at Release 1,
// show the provenance of their own Release — not of another Library's Release
// of the same number (task 72_5).
func TestLibraryPin_ProvenanceOfEachLibrary(t *testing.T) {
	ctx := context.Background()
	svc := newLibraryPinService()
	first, second := mustCreateLibrary(t), mustCreateLibrary(t)
	mustReleaseLibraryTitled(t, first.ID, "First library", "Ada")
	mustReleaseLibraryTitled(t, second.ID, "Second library", "Bob")
	for _, lib := range []appdto.Library{first, second} {
		if _, _, err := svc.PutLibraryPin(ctx, mainProject, lib.ID, 1, draftVersionOf(t, mainProject)); err != nil {
			t.Fatalf("pin library %s: %v", lib.ID, err)
		}
	}

	pins, err := svc.FindLibraryPins(ctx, mainProject)
	if err != nil {
		t.Fatalf("FindLibraryPins: %v", err)
	}
	want := map[uuid.UUID]struct{ title, author string }{
		first.ID:  {"First library", "Ada"},
		second.ID: {"Second library", "Bob"},
	}
	seen := map[uuid.UUID]bool{}
	for _, pin := range pins {
		expected, ok := want[pin.LibraryID]
		if !ok {
			continue
		}
		seen[pin.LibraryID] = true
		if pin.Title != expected.title || pin.Author != expected.author || pin.CommitNumber == nil || *pin.CommitNumber != 1 {
			t.Errorf("the pin of library %s: %+v, want commit 1 titled %q by %s", pin.LibraryID, pin, expected.title, expected.author)
		}
	}
	if len(seen) != len(want) {
		t.Errorf("the pins found: %v, want both libraries", seen)
	}
	for _, lib := range []appdto.Library{first, second} {
		if _, err := svc.DeleteLibraryPin(ctx, mainProject, lib.ID, draftVersionOf(t, mainProject)); err != nil {
			t.Fatalf("unpin library %s: %v", lib.ID, err)
		}
	}
}
