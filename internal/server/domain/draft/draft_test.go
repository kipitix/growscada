package draft

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

func makeDraft(t *testing.T) Draft {
	t.Helper()
	scope := ProjectScope(id.NewID[project.Project]())
	return NewDraft(id.NewID[Draft](), scope, version.Committed[Draft](), version.Initial[Draft](), 3)
}

func TestDraft_Metadata(t *testing.T) {
	d := makeDraft(t)
	if d.Version() != version.Committed[Draft]() || d.StackPosition() != 3 {
		t.Errorf("draft %s at %d", d.Version(), d.StackPosition())
	}
	if d.Scope().IsProject() == d.Scope().IsLibrary() {
		t.Errorf("the scope names exactly one Draft: %s", d.Scope())
	}
}

func TestDraft_WithVersionAndStackPosition(t *testing.T) {
	d := makeDraft(t)
	next := d.WithVersion(version.Committed[Draft]().Next()).WithStackPosition(4)
	if next.Version().Number() != 2 || next.StackPosition() != 4 {
		t.Errorf("the copy is at %d/%d", next.Version().Number(), next.StackPosition())
	}
	if d.Version().Number() != 1 || d.StackPosition() != 3 {
		t.Errorf("the original changed: %d/%d", d.Version().Number(), d.StackPosition())
	}
}

// TestDraft_HasUnsavedChanges: the Draft is unsaved exactly while its Version is
// above its Clean Version — a content write raises the Version, a Save or a
// clean replacement moves the marker, and an undo raises the Version alone
// (task 74; ADR 0016, amendment 2026-11-10).
func TestDraft_HasUnsavedChanges(t *testing.T) {
	d := makeDraft(t) // version 1, clean 0: written, never saved
	if !d.HasUnsavedChanges() {
		t.Fatalf("a Draft written but never saved is clean: %s/%s", d.Version(), d.CleanVersion())
	}
	if got := d.WithCleanVersion(d.Version()); got.HasUnsavedChanges() {
		t.Errorf("a Draft at its Clean Version reports unsaved changes: %s/%s", got.Version(), got.CleanVersion())
	}
	// A content write: the Version rises, the marker stays.
	written := d.WithVersion(d.Version().Next())
	if !written.HasUnsavedChanges() {
		t.Errorf("a write above the Clean Version is clean")
	}
	// Undo raises the Version alone, so a Draft at its Clean Version becomes
	// unsaved by it.
	undone, err := d.WithCleanVersion(d.Version()).Undo(DraftChange{Seq: 3})
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if !undone.HasUnsavedChanges() {
		t.Errorf("an undo above the Clean Version is clean")
	}
	if d.CleanVersion().Number() != 0 {
		t.Errorf("the original changed: clean %s", d.CleanVersion())
	}
}

// TestDraft_WithCleanVersionKeepsTheOriginal: WithCleanVersion returns a copy,
// as WithVersion does.
func TestDraft_WithCleanVersionKeepsTheOriginal(t *testing.T) {
	d := makeDraft(t)
	clean := d.WithCleanVersion(d.Version())
	if clean.CleanVersion() != d.Version() || d.CleanVersion().Number() != 0 {
		t.Errorf("the original's Clean Version changed: %s", d.CleanVersion())
	}
}
