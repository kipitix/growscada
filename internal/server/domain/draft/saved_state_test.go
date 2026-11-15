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

func TestSavedStateTitle_TrimsAndRefusesEmptyOrLong(t *testing.T) {
	title, err := draft.NewSavedStateTitle("  Before the demo  ")
	if err != nil {
		t.Fatalf("NewSavedStateTitle: %v", err)
	}
	if title.String() != "Before the demo" {
		t.Errorf("title %q, want it trimmed", title)
	}
	if _, err := draft.NewSavedStateTitle("   "); err == nil {
		t.Error("an empty title was accepted")
	}
	long := make([]rune, 201)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := draft.NewSavedStateTitle(string(long)); err == nil {
		t.Error("a title of 201 characters was accepted")
	}
	if _, err := draft.NewSavedStateTitle(string(long[:200])); err != nil {
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

func TestNewSavedState_RecordsCreatedAndWithTitleRecordsUpdated(t *testing.T) {
	projectID := id.NewID[project.Project]()
	scope := draft.ProjectScope(projectID)
	at := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	title, _ := draft.NewSavedStateTitle("Before the demo")
	author, _ := draft.NewAuthor("Ada")
	ver, _ := version.New(version.WithNumber[draft.Draft](7))
	content := draft.NewProjectSavedStateContent(projectv0.ProjectFile{
		Header: projectv0.Header{SchemaVersion: projectv0.SchemaVersion(), ProjectID: projectID.UUID()},
		Tags:   []projectv0.TagDefinition{},
	})

	savedState, err := draft.NewSavedState(scope, id.NewID[draft.SavedState](), 1, title, author, at, ver, content)
	if err != nil {
		t.Fatalf("NewSavedState: %v", err)
	}
	events := savedState.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeSavedStateCreated {
		t.Fatalf("events %v, want one commit_created", events)
	}
	if savedState.DraftVersion().Number() != 7 {
		t.Errorf("saved state fixed version %s, want 7", savedState.DraftVersion())
	}
	if savedState.Content().IsProject() != true || savedState.Content().IsLibrary() {
		t.Errorf("content is not a project file")
	}

	renamedTitle, _ := draft.NewSavedStateTitle("For the demo")
	renamed := savedState.WithTitle(renamedTitle)
	if renamed.Title().String() != "For the demo" {
		t.Errorf("renamed title %q", renamed.Title())
	}
	if renamed.Number() != savedState.Number() || renamed.Content().IsProject() != true {
		t.Errorf("the rename touched the snapshot")
	}
	events = renamed.PendingEvents()
	if len(events) != 1 || events[0].Type() != event.EventTypeSavedStateTitleUpdated {
		t.Fatalf("events %v, want one commit_title_updated", events)
	}
}

func TestNewSavedState_RefusesContentOfTheOtherScope(t *testing.T) {
	title, _ := draft.NewSavedStateTitle("Title")
	author, _ := draft.NewAuthor("Ada")
	ver, _ := version.New(version.WithNumber[draft.Draft](1))
	types := draft.NewLibrarySavedStateContent([]library.WidgetTypeCopy{})
	if _, err := draft.NewSavedState(draft.ProjectScope(id.NewID[project.Project]()),
		id.NewID[draft.SavedState](), 1, title, author, time.Now(), ver, types); err == nil {
		t.Error("a project's saved state with widget types was accepted")
	}
	file := draft.NewProjectSavedStateContent(projectv0.ProjectFile{})
	if _, err := draft.NewSavedState(draft.LibraryScope(id.NewID[library.Library]()),
		id.NewID[draft.SavedState](), 1, title, author, time.Now(), ver, file); err == nil {
		t.Error("a library's saved state with a project file was accepted")
	}
}

func TestSavedStateContent_SurvivesTheEncoding(t *testing.T) {
	libraryID := id.NewID[library.Library]()
	typeID := id.NewID[library.WidgetType]()
	name, _ := library.NewWidgetTypeName("gauge")
	html, _ := library.NewHtmlTemplate("<div></div>")
	script, _ := library.NewScript("function update(){}")
	portName, _ := library.NewInputPortName("speed")
	hint, _ := library.NewPortTypeHint("integer")
	wt, err := library.NewWidgetTypeCopy(typeID, libraryID, name, html, script,
		library.ScriptLanguageJavaScript, library.DefaultSize(), []library.InputPort{
			library.NewInputPort(portName, "speed", hint),
		})
	if err != nil {
		t.Fatalf("NewWidgetTypeCopy: %v", err)
	}

	encoded, err := json.Marshal(draft.NewLibrarySavedStateContent([]library.WidgetTypeCopy{wt}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded draft.SavedStateContent
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	types, ok := decoded.WidgetTypes()
	if !ok || len(types) != 1 || types[0].ID() != typeID || types[0].InputPorts()[0].Name() != portName {
		t.Errorf("the widget types did not survive the encoding: %+v", types)
	}
}
