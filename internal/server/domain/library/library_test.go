package library

import (
	"errors"
	"testing"
	"time"

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
	return ReconstituteLibrary(testLibraryID, id.NewID[organization.Organization](), mustLibraryName(t, "Main"), false, ReleaseNumber{}, ver)
}

func TestCreateLibrary_RecordsCreatedEventAtInitialVersion(t *testing.T) {
	lib := CreateLibrary(testLibraryID, id.NewID[organization.Organization](), mustLibraryName(t, "Main"))
	if lib.Version() != version.Initial[Library]() || lib.IsDeleted() {
		t.Errorf("expected a live library at the initial version, got %s", lib)
	}
	if _, discarding := lib.DiscardingTo(); discarding {
		t.Error("a new library is not being discarded")
	}
	if got := pendingTypes(lib); len(got) != 1 || got[0] != event.EventTypeLibraryCreated {
		t.Errorf("expected [library_created], got %v", got)
	}
}

func TestLibraryRename_StaleVersionIsConflict(t *testing.T) {
	lib := makeStoredLibrary(t)
	if _, err := lib.Rename(version.Committed[Library](), mustLibraryName(t, "Other")); !errors.Is(err, ErrLibraryConflict) {
		t.Errorf("expected ErrLibraryConflict, got %v", err)
	}
	renamed, err := lib.Rename(lib.Version(), mustLibraryName(t, "Other"))
	if err != nil || renamed.Name().String() != "Other" || lib.Name().String() != "Main" {
		t.Fatalf("Rename: %v, %s", err, renamed)
	}
	if got := pendingTypes(renamed); len(got) != 1 || got[0] != event.EventTypeLibraryUpdated {
		t.Errorf("expected [library_updated], got %v", got)
	}
}

func TestLibraryStartDiscard_MarksUntilFinished(t *testing.T) {
	lib := makeStoredLibrary(t)
	if _, err := lib.StartDiscard(ReleaseNumber{}); !errors.Is(err, ErrNoRelease) {
		t.Errorf("a library never released: expected ErrNoRelease, got %v", err)
	}
	two, _ := NewReleaseNumber(2)
	marked, err := lib.StartDiscard(two)
	if err != nil {
		t.Fatalf("StartDiscard: %v", err)
	}
	if to, ok := marked.DiscardingTo(); !ok || to != two {
		t.Errorf("DiscardingTo = %s, %t; want 2, true", to, ok)
	}
	if !errors.Is(marked.CheckEditable(), ErrLibraryDiscarding) {
		t.Error("the Draft of a library being discarded is read-only")
	}
	if _, err := marked.StartDiscard(two); !errors.Is(err, ErrLibraryDiscarding) {
		t.Errorf("a second Discard: expected ErrLibraryDiscarding, got %v", err)
	}
	if got := pendingTypes(marked); len(got) != 1 || got[0] != event.EventTypeLibraryDiscardStarted {
		t.Errorf("expected [library_discard_started], got %v", got)
	}

	done := marked.FinishDiscard()
	if _, ok := done.DiscardingTo(); ok || done.CheckEditable() != nil {
		t.Errorf("FinishDiscard left the mark: %s", done)
	}
	if got := pendingTypes(done); len(got) != 2 || got[1] != event.EventTypeLibraryUpdated {
		t.Errorf("expected [library_discard_started library_updated], got %v", got)
	}
	if lib.FinishDiscard() != lib {
		t.Error("FinishDiscard of a library not being discarded must change nothing")
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

func TestCreateRelease_NumbersAndRefusesAnUnchangedDraft(t *testing.T) {
	lib := makeStoredLibrary(t)
	draft := []WidgetType{makeCommittedWidgetType(t)}
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

	first, err := CreateRelease(lib, draft, nil, "first", at)
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if first.Number() != FirstRelease() || first.Note() != "first" || !first.CreatedAt().Equal(at) || len(first.WidgetTypes()) != 1 {
		t.Errorf("unexpected first release %s", first)
	}
	if ev, ok := first.PendingEvents()[0].(LibraryReleasedEvent); !ok || ev.Release() != FirstRelease() || ev.LibraryID() != lib.ID() {
		t.Errorf("expected library_released of release 1, got %v", first.PendingEvents())
	}

	if _, err := CreateRelease(lib, draft, first, "", at); !errors.Is(err, ErrNothingToRelease) {
		t.Errorf("an unchanged Draft: expected ErrNothingToRelease, got %v", err)
	}

	changed, err := draft[0].Update(draft[0].Version(), mustName(t, "thermometer"), draft[0].HtmlTemplate(), draft[0].Script(), draft[0].ScriptLanguage(), draft[0].DefaultSize(), nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	second, err := CreateRelease(lib, []WidgetType{changed}, first, "", at)
	if err != nil || second.Number().Int() != 2 {
		t.Fatalf("second release: %v, %v", second, err)
	}

	marked, _ := lib.StartDiscard(first.Number())
	if _, err := CreateRelease(marked, []WidgetType{changed}, first, "", at); !errors.Is(err, ErrLibraryDiscarding) {
		t.Errorf("a Release during a Discard: expected ErrLibraryDiscarding, got %v", err)
	}
}

func TestDraftDiffers(t *testing.T) {
	lib := makeStoredLibrary(t)
	wt := makeCommittedWidgetType(t)
	release, err := CreateRelease(lib, []WidgetType{wt}, nil, "", time.Now())
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	renamed, _ := wt.Update(wt.Version(), mustName(t, "other"), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.DefaultSize(), nil)
	port, _ := NewInputPortName("value")
	withPort, _ := wt.Update(wt.Version(), wt.Name(), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.DefaultSize(),
		[]InputPort{NewInputPort(port, "", AnyTypeHint())})

	cases := map[string]struct {
		draft []WidgetType
		want  bool
	}{
		"same":                    {[]WidgetType{wt}, false},
		"same at a newer version": {[]WidgetType{reconstituteAt(t, wt, 5)}, false},
		"renamed":                 {[]WidgetType{renamed}, true},
		"port added":              {[]WidgetType{withPort}, true},
		"removed":                 {nil, true},
		"added":                   {[]WidgetType{wt, makeCommittedWidgetType(t)}, true},
	}
	for name, tc := range cases {
		if got := DraftDiffers(tc.draft, release); got != tc.want {
			t.Errorf("%s: DraftDiffers = %t, want %t", name, got, tc.want)
		}
	}
	if !DraftDiffers(nil, nil) {
		t.Error("a Library never released differs from any Draft")
	}
}

func TestReleaseStateOf(t *testing.T) {
	lib := makeStoredLibrary(t)
	wt := makeCommittedWidgetType(t)
	if got := ReleaseStateOf([]WidgetType{wt}, nil); got.Latest.IsValid() || !got.DraftDiffers {
		t.Errorf("never released: %+v", got)
	}
	release, err := CreateRelease(lib, []WidgetType{wt}, nil, "", time.Now())
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if got := ReleaseStateOf([]WidgetType{wt}, release); got.Latest != FirstRelease() || got.DraftDiffers {
		t.Errorf("released: %+v", got)
	}
	if got := ReleaseStateOf(nil, release); got.Latest != FirstRelease() || !got.DraftDiffers {
		t.Errorf("changed since: %+v", got)
	}
}

func TestWidgetTypeResetTo_RestoresTheReleasedDefinition(t *testing.T) {
	wt := makeCommittedWidgetType(t)
	released := ReleasedFrom(wt)
	changed, _ := wt.Update(wt.Version(), mustName(t, "other"), wt.HtmlTemplate(), wt.Script(), ScriptLanguageLua, wt.DefaultSize(), nil)

	reset, err := reconstituteAt(t, changed, 3).ResetTo(released)
	if err != nil {
		t.Fatalf("ResetTo: %v", err)
	}
	if !released.Matches(reset) {
		t.Errorf("ResetTo did not restore the release: %s", reset)
	}
	if got := pendingTypes(reset); len(got) != 1 || got[0] != event.EventTypeWidgetTypeUpdated {
		t.Errorf("expected [widget_type_updated], got %v", got)
	}
	if _, err := makeCommittedWidgetType(t).ResetTo(released); err == nil {
		t.Error("ResetTo the copy of another WidgetType must fail")
	}

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

// reconstituteAt returns wt as stored at version n.
func reconstituteAt(t *testing.T, wt WidgetType, n int) WidgetType {
	t.Helper()
	ver, _ := version.New(version.WithNumber[WidgetType](n))
	stored, err := ReconstituteWidgetType(wt.ID(), wt.LibraryID(), wt.Name(), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.DefaultSize(), wt.InputPorts(), ver)
	if err != nil {
		t.Fatalf("ReconstituteWidgetType: %v", err)
	}
	return stored
}
