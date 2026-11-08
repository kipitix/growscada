package release_test

import (
	"strings"
	"testing"
	"time"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/release"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// projectScope is the scope of the tests.
func projectScope() draft.Scope {
	return draft.ProjectScope(id.NewID[project.Project]())
}

// testCommit returns a Commit of scope number 3 with a ProjectFile.
func testCommit(t *testing.T, scope draft.Scope) draft.Commit {
	t.Helper()
	title, err := draft.NewCommitTitle("Before the demo")
	if err != nil {
		t.Fatalf("NewCommitTitle: %v", err)
	}
	author, err := draft.NewAuthor("Ada")
	if err != nil {
		t.Fatalf("NewAuthor: %v", err)
	}
	file := projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: scope.ProjectID().UUID()},
		LibraryPins: []projectv0.LibraryPin{
			{LibraryID: id.NewID[library.Library]().UUID(), Release: 2},
		},
	}
	commit, err := draft.NewCommit(scope, id.NewID[draft.Commit](), 3, title, author, time.Now().UTC(),
		version.Committed[draft.Draft](), draft.NewProjectCommitContent(file))
	if err != nil {
		t.Fatalf("NewCommit: %v", err)
	}
	return commit
}

// TestCreateRelease_CopiesTheCommitAndRecordsCreated: a Release copies the
// Commit's number, title and content, takes its own number and note, and
// records release_created (task 72_3; ADR 0021).
func TestCreateRelease_CopiesTheCommitAndRecordsCreated(t *testing.T) {
	scope := projectScope()
	commit := testCommit(t, scope)
	number, err := release.NewNumber(1)
	if err != nil {
		t.Fatalf("NewNumber: %v", err)
	}
	note, err := release.NewNote("  for the demo  ")
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}
	author, err := draft.NewAuthor("Grace")
	if err != nil {
		t.Fatalf("NewAuthor: %v", err)
	}

	created, err := release.CreateRelease(scope, commit, number, note, author, time.Now().UTC())
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if created.Number() != number || created.CommitNumber() != commit.Number() {
		t.Errorf("release %s of commit %d", created.Number(), created.CommitNumber())
	}
	if created.CommitTitle().String() != "Before the demo" {
		t.Errorf("the title snapshot is %q", created.CommitTitle())
	}
	if created.Note().String() != "for the demo" {
		t.Errorf("the note is %q", created.Note())
	}
	if created.Origin() != release.OriginCommit {
		t.Errorf("the origin is %s", created.Origin())
	}
	if file, ok := created.Content().ProjectFile(); !ok || file.ProjectID != scope.ProjectID().UUID() {
		t.Errorf("the content is not the commit's project file: %+v", created.Content())
	}
	events := created.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeReleaseCreated {
		t.Errorf("the recorded events: %v", events)
	}
}

// TestCreateRelease_RefusesACommitOfAnotherScope: a Commit of another Project
// cannot be released into this one.
func TestCreateRelease_RefusesACommitOfAnotherScope(t *testing.T) {
	scope := projectScope()
	commit := testCommit(t, projectScope())
	number, _ := release.NewNumber(1)
	note, _ := release.NewNote("")
	author, _ := draft.NewAuthor("Ada")
	if _, err := release.CreateRelease(scope, commit, number, note, author, time.Now().UTC()); err == nil {
		t.Fatal("a commit of another project was released")
	}
}

// TestWithNote_RecordsNoteUpdatedAndKeepsTheContent: a note changes on its own
// and records release_note_updated; the content snapshot is untouched.
func TestWithNote_RecordsNoteUpdatedAndKeepsTheContent(t *testing.T) {
	scope := projectScope()
	commit := testCommit(t, scope)
	number, _ := release.NewNumber(2)
	note, _ := release.NewNote("")
	author, _ := draft.NewAuthor("Ada")
	created, err := release.CreateRelease(scope, commit, number, note, author, time.Now().UTC())
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	changed, _ := release.NewNote("renamed")
	updated := created.WithNote(changed)
	if updated.Note().String() != "renamed" {
		t.Errorf("the note is %q", updated.Note())
	}
	events := updated.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeReleaseNoteUpdated {
		t.Errorf("the recorded events: %v", events)
	}
	if before, _ := created.Content().ProjectFile(); before.ProjectID != scope.ProjectID().UUID() {
		t.Errorf("the content changed with the note")
	}
}

// TestNote_Rules: an empty note is allowed, a note longer than the limit is
// not.
func TestNote_Rules(t *testing.T) {
	if note, err := release.NewNote("  "); err != nil || note.String() != "" {
		t.Errorf("an empty note: %q, %v", note, err)
	}
	if _, err := release.NewNote(strings.Repeat("x", 2001)); err == nil {
		t.Error("an over-long note was accepted")
	}
}

// TestOrigin_Parses: the origins are its names and the zero value is invalid
// (ADR 0001).
func TestOrigin_Parses(t *testing.T) {
	for _, want := range []release.Origin{release.OriginCommit, release.OriginFile} {
		got, err := release.NewOrigin(want.String())
		if err != nil || got != want {
			t.Errorf("NewOrigin(%q) = %s, %v", want, got, err)
		}
	}
	var zeroOrigin release.Origin
	if zeroOrigin.IsValid() {
		t.Error("the zero origin is valid")
	}
	if _, err := release.NewOrigin("legacy"); err == nil {
		t.Error("an unknown origin was accepted")
	}
}
