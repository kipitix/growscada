package project

import (
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestPinProvenance_CommitAndLegacy: a pin shows the Commit its Release
// published, with that title, the Author and the note; a Release that predates
// Commits says so (task 72_5).
func TestPinProvenance_CommitAndLegacy(t *testing.T) {
	p := &Project{}
	number, title := 3, "Widgets of the release"
	got := p.pinProvenance(apiv0.LibraryPinResponse{
		Release: 2, CommitNumber: &number, Title: &title, Author: "Ada", Origin: apiv0.LibraryReleaseOriginCommit, Note: "a note",
	})
	if want := "commit #3 Widgets of the release · Ada · a note"; got != want {
		t.Errorf("the provenance: %q, want %q", got, want)
	}
	legacy := p.pinProvenance(apiv0.LibraryPinResponse{Release: 1, Origin: apiv0.LibraryReleaseOriginLegacy})
	if legacy != "legacy release" {
		t.Errorf("the legacy provenance: %q", legacy)
	}
}
