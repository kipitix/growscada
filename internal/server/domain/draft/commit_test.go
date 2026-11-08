package draft_test

import (
	"encoding/json"
	"testing"
	"time"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

func TestCommitTitle_TrimsAndRefusesEmptyOrLong(t *testing.T) {
	title, err := draft.NewCommitTitle("  Before the demo  ")
	if err != nil {
		t.Fatalf("NewCommitTitle: %v", err)
	}
	if title.String() != "Before the demo" {
		t.Errorf("title %q, want it trimmed", title)
	}
	if _, err := draft.NewCommitTitle("   "); err == nil {
		t.Error("an empty title was accepted")
	}
	long := make([]rune, 201)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := draft.NewCommitTitle(string(long)); err == nil {
		t.Error("a title of 201 characters was accepted")
	}
	if _, err := draft.NewCommitTitle(string(long[:200])); err != nil {
		t.Errorf("a title of 200 characters was refused: %v", err)
	}
}

func TestAuthor_TrimsAndRefusesEmptyOrLong(t *testing.T) {
	author, err := draft.NewAuthor("  Ada  ")
	if err != nil {
		t.Fatalf("NewAuthor: %v", err)
	}
	if author.String() != "Ada" {
		t.Errorf("author %q, want it trimmed", author)
	}
	if _, err := draft.NewAuthor(""); err == nil {
		t.Error("an empty author was accepted")
	}
	long := make([]rune, 101)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := draft.NewAuthor(string(long)); err == nil {
		t.Error("an author of 101 characters was accepted")
	}
}

func TestNewCommit_RecordsCreatedAndWithTitleRecordsUpdated(t *testing.T) {
	projectID := id.NewID[project.Project]()
	scope := draft.ProjectScope(projectID)
	at := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	title, _ := draft.NewCommitTitle("Before the demo")
	author, _ := draft.NewAuthor("Ada")
	ver, _ := version.New(version.WithNumber[draft.Draft](7))
	content := draft.NewProjectCommitContent(projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID.UUID()},
		Tags:   []projectv0.TagDefinition{},
	})

	commit, err := draft.NewCommit(scope, id.NewID[draft.Commit](), 1, title, author, at, ver, content)
	if err != nil {
		t.Fatalf("NewCommit: %v", err)
	}
	events := commit.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeCommitCreated {
		t.Fatalf("events %v, want one commit_created", events)
	}
	if commit.DraftVersion().Number() != 7 {
		t.Errorf("commit fixed version %s, want 7", commit.DraftVersion())
	}
	if commit.Content().IsProject() != true || commit.Content().IsLibrary() {
		t.Errorf("content is not a project file")
	}

	renamedTitle, _ := draft.NewCommitTitle("For the demo")
	renamed := commit.WithTitle(renamedTitle)
	if renamed.Title().String() != "For the demo" {
		t.Errorf("renamed title %q", renamed.Title())
	}
	if renamed.Number() != commit.Number() || renamed.Content().IsProject() != true {
		t.Errorf("the rename touched the snapshot")
	}
	events = renamed.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeCommitTitleUpdated {
		t.Fatalf("events %v, want one commit_title_updated", events)
	}
}

func TestNewCommit_RefusesContentOfTheOtherScope(t *testing.T) {
	title, _ := draft.NewCommitTitle("Title")
	author, _ := draft.NewAuthor("Ada")
	ver, _ := version.New(version.WithNumber[draft.Draft](1))
	types := draft.NewLibraryCommitContent([]library.ReleasedWidgetType{})
	if _, err := draft.NewCommit(draft.ProjectScope(id.NewID[project.Project]()),
		id.NewID[draft.Commit](), 1, title, author, time.Now(), ver, types); err == nil {
		t.Error("a project's commit with widget types was accepted")
	}
	file := draft.NewProjectCommitContent(projectv0.ProjectFile{})
	if _, err := draft.NewCommit(draft.LibraryScope(id.NewID[library.Library]()),
		id.NewID[draft.Commit](), 1, title, author, time.Now(), ver, file); err == nil {
		t.Error("a library's commit with a project file was accepted")
	}
}

func TestCommitContent_SurvivesTheEncoding(t *testing.T) {
	libraryID := id.NewID[library.Library]()
	typeID := id.NewID[library.WidgetType]()
	name, _ := library.NewWidgetTypeName("gauge")
	html, _ := library.NewHtmlTemplate("<div></div>")
	script, _ := library.NewScript("function update(){}")
	portName, _ := library.NewInputPortName("speed")
	hint, _ := library.NewPortTypeHint("integer")
	wt, err := library.NewReleasedWidgetType(typeID, libraryID, name, html, script,
		library.ScriptLanguageJavaScript, library.DefaultSize(), []library.InputPort{
			library.NewInputPort(portName, "speed", hint),
		})
	if err != nil {
		t.Fatalf("NewReleasedWidgetType: %v", err)
	}

	encoded, err := json.Marshal(draft.NewLibraryCommitContent([]library.ReleasedWidgetType{wt}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded draft.CommitContent
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	types, ok := decoded.WidgetTypes()
	if !ok || len(types) != 1 || types[0].ID() != typeID || types[0].InputPorts()[0].Name() != portName {
		t.Errorf("the widget types did not survive the encoding: %+v", types)
	}
}
