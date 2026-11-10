package library

import (
	"errors"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/description"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

func mustLibraryName(t *testing.T, s string) LibraryName {
	t.Helper()
	name, err := NewLibraryName(s)
	if err != nil {
		t.Fatalf("NewLibraryName(%q): %v", s, err)
	}
	return name
}

// makeStoredLibrary builds a Library as read back from a repository.
func makeStoredLibrary(t *testing.T) Library {
	t.Helper()
	ver, _ := version.New(version.WithNumber[Library](2))
	return ReconstituteLibrary(testLibraryID, id.NewID[organization.Organization](), mustLibraryName(t, "Main"), description.Description{}, false, ver)
}

func TestCreateLibrary_RecordsCreatedEventAtInitialVersion(t *testing.T) {
	lib := CreateLibrary(testLibraryID, id.NewID[organization.Organization](), mustLibraryName(t, "Main"), description.FromStorage("A shared library"))
	if lib.Version() != version.Initial[Library]() || lib.IsDeleted() {
		t.Errorf("expected a live library at the initial version, got %s", lib)
	}
	if got := pendingTypes(lib); len(got) != 1 || got[0] != event.EventTypeLibraryCreated {
		t.Errorf("expected [library_created], got %v", got)
	}
}

func TestLibraryUpdate_ChangesTheNameAndDescription(t *testing.T) {
	lib := makeStoredLibrary(t)
	if _, err := lib.Update(version.Committed[Library](), mustLibraryName(t, "Other"), description.Description{}); !errors.Is(err, ErrLibraryConflict) {
		t.Errorf("expected ErrLibraryConflict, got %v", err)
	}
	updated, err := lib.Update(lib.Version(), mustLibraryName(t, "Other"), description.FromStorage("Changed"))
	if err != nil || updated.Name().String() != "Other" || updated.Description().String() != "Changed" || lib.Name().String() != "Main" {
		t.Fatalf("Update: %v, %s", err, updated)
	}
	if got := pendingTypes(updated); len(got) != 1 || got[0] != event.EventTypeLibraryUpdated {
		t.Errorf("expected [library_updated], got %v", got)
	}
}

func TestLibraryDelete_MarksDeletedAndRecordsDeletedEvent(t *testing.T) {
	lib := makeStoredLibrary(t)
	deleted := lib.Delete()
	if !deleted.IsDeleted() || lib.IsDeleted() {
		t.Errorf("Delete: %s (original %s)", deleted, lib)
	}
	if got := pendingTypes(deleted); len(got) != 1 || got[0] != event.EventTypeLibraryDeleted {
		t.Errorf("expected [library_deleted], got %v", got)
	}
}

func TestCreateRelease_FromACommit(t *testing.T) {
	lib := makeStoredLibrary(t)
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	author, err := NewAuthor("Ada")
	if err != nil {
		t.Fatalf("NewAuthor: %v", err)
	}

	first, err := CreateRelease(lib, FirstRelease(), 1, "first", author, at, nil)
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	commitNumber := first.CommitNumber()
	if first.Number() != FirstRelease() || first.Note() != "first" || !first.CreatedAt().Equal(at) ||
		commitNumber == nil || *commitNumber != 1 || first.Author().String() != "Ada" || first.Origin() != OriginCommit {
		t.Errorf("unexpected first release %s: %+v", first, first)
	}
	if ev, ok := first.PendingEvents()[0].(LibraryReleasedEvent); !ok || ev.Release() != FirstRelease() || ev.LibraryID() != lib.ID() {
		t.Errorf("expected library_released of release 1, got %v", first.PendingEvents())
	}

	second, err := CreateRelease(lib, FirstRelease().Next(), 2, "", author, at, nil)
	if err != nil || second.Number().Int() != 2 {
		t.Fatalf("second release: %v, %v", second, err)
	}
}

func TestCreateRelease_RefusesAWidgetTypeOfAnotherLibrary(t *testing.T) {
	lib := makeStoredLibrary(t)
	author, _ := NewAuthor("Ada")
	other := ReleasedFrom(makeCommittedWidgetType(t))
	other.libraryID = id.NewID[Library]()
	if _, err := CreateRelease(lib, FirstRelease(), 1, "", author, time.Now(), []ReleasedWidgetType{other}); err == nil {
		t.Error("a WidgetType of another Library: expected an error")
	}
}

func TestReconstituteRelease_ChecksTheProvenance(t *testing.T) {
	author, _ := NewAuthor("Ada")
	at := time.Now()
	commit := 1
	if _, err := ReconstituteRelease(testLibraryID, FirstRelease(), nil, author, OriginCommit, "", at, nil); err == nil {
		t.Error("a release of a commit without its number: expected an error")
	}
	if _, err := ReconstituteRelease(testLibraryID, FirstRelease(), &commit, author, OriginLegacy, "", at, nil); err == nil {
		t.Error("a legacy release naming a commit: expected an error")
	}
	legacy, err := ReconstituteRelease(testLibraryID, FirstRelease(), nil, author, OriginLegacy, "", at, nil)
	if err != nil || legacy.CommitNumber() != nil || legacy.Origin() != OriginLegacy {
		t.Errorf("legacy release: %v, %v", legacy, err)
	}
}

func TestReleaseWithNote_RecordsNoteUpdatedAndKeepsTheContent(t *testing.T) {
	lib := makeStoredLibrary(t)
	author, _ := NewAuthor("Ada")
	release, err := CreateRelease(lib, FirstRelease(), 1, "first", author, time.Now(), nil)
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	updated := release.WithNote("second")
	if updated.Note() != "second" || release.Note() != "first" {
		t.Errorf("WithNote: %q, original %q", updated.Note(), release.Note())
	}
	if updated.Number() != release.Number() || updated.CommitNumber() == nil || *updated.CommitNumber() != 1 {
		t.Errorf("WithNote changed the identity: %+v", updated)
	}
	ev, ok := updated.PendingEvents()[0].(LibraryReleaseNoteUpdatedEvent)
	if !ok || ev.Release() != FirstRelease() || ev.LibraryID() != lib.ID() {
		t.Errorf("expected library_release_note_updated, got %v", updated.PendingEvents())
	}
}

func TestReleaseStateOf(t *testing.T) {
	author, _ := NewAuthor("Ada")
	commit := 2
	released, err := ReconstituteRelease(testLibraryID, FirstRelease(), &commit, author, OriginCommit, "", time.Now(), nil)
	if err != nil {
		t.Fatalf("ReconstituteRelease: %v", err)
	}
	legacy, err := ReconstituteRelease(testLibraryID, FirstRelease(), nil, author, OriginLegacy, "", time.Now(), nil)
	if err != nil {
		t.Fatalf("ReconstituteRelease: %v", err)
	}

	cases := map[string]struct {
		latest             LibraryRelease
		lastCommit         int
		unsaved            bool
		wantLatest         int
		wantLastReleaseCmt int
		wantUnpublished    bool
	}{
		"never saved, never released": {nil, 0, false, 0, 0, false},
		"saved, not released":         {nil, 2, false, 0, 0, true},
		"released commit 2":           {released, 2, false, 1, 2, false},
		"moved after the commit":      {released, 3, true, 1, 2, true},
		"legacy release, no commit":   {legacy, 0, true, 1, 0, false},
		"legacy release, commit 1":    {legacy, 1, false, 1, 0, true},
	}
	for name, tc := range cases {
		got := ReleaseStateOf(tc.latest, tc.lastCommit, tc.unsaved)
		if got.Latest.Int() != tc.wantLatest || got.LatestReleaseCommit != tc.wantLastReleaseCmt ||
			got.UnsavedChanges != tc.unsaved || got.UnpublishedChanges != tc.wantUnpublished {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestRecreateWidgetType_RestoresTheReleasedDefinition(t *testing.T) {
	wt := makeCommittedWidgetType(t)
	released := ReleasedFrom(wt)

	recreated, err := RecreateWidgetType(released)
	if err != nil || recreated.ID() != wt.ID() || recreated.LibraryID() != wt.LibraryID() || !released.Matches(recreated) {
		t.Fatalf("RecreateWidgetType: %v, %s", err, recreated)
	}
	if got := pendingTypes(recreated); len(got) != 1 || got[0] != event.EventTypeWidgetTypeCreated {
		t.Errorf("expected [widget_type_created], got %v", got)
	}
}

func mustName(t *testing.T, s string) WidgetTypeName {
	t.Helper()
	name, err := NewWidgetTypeName(s)
	if err != nil {
		t.Fatalf("NewWidgetTypeName(%q): %v", s, err)
	}
	return name
}
