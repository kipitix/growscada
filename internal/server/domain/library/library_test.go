package library

import (
	"errors"
	"testing"

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

// TestWidgetTypeCopy_SameAs: two copies are the same exactly while their
// definitions are (task 74; ADR 0016, amendment 2026-11-10).
func TestWidgetTypeCopy_SameAs(t *testing.T) {
	wt := makeCommittedWidgetType(t)
	released := CopyOf(wt)
	if !released.SameAs(CopyOf(wt)) || !released.Matches(wt) {
		t.Fatalf("a copy of the same WidgetType differs: %s", released)
	}
	if released.SameAs(CopyOf(makeCommittedWidgetType(t))) {
		t.Errorf("two WidgetTypes with different IDs are the same")
	}
	html, err := NewHtmlTemplate("<span></span>")
	if err != nil {
		t.Fatalf("NewHtmlTemplate: %v", err)
	}
	changed, err := ReconstituteWidgetType(wt.ID(), wt.LibraryID(), wt.Name(), html, wt.Script(),
		wt.ScriptLanguage(), wt.DefaultSize(), wt.InputPorts())
	if err != nil {
		t.Fatalf("ReconstituteWidgetType: %v", err)
	}
	if released.SameAs(CopyOf(changed)) {
		t.Errorf("copies with different templates are the same")
	}
}

func TestRecreateWidgetType_RestoresTheReleasedDefinition(t *testing.T) {
	wt := makeCommittedWidgetType(t)
	released := CopyOf(wt)

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
