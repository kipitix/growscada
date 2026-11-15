package library

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
)

// The Library's Save flow of task 72_7: Ctrl+S, the status of the list, a load
// of a saved state and a Discard made in another tab.

// saveWithUI fixes the Draft as a Commit through the Save dialog and consumes
// its confirmation toast.
func saveWithUI(t *testing.T, f *fixture, title string) {
	t.Helper()
	f.l.saveAuthor, f.l.saveTitle = "Ada", title
	f.l.saveCommit()
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "saved") {
		t.Fatalf("the Save confirmation: %+v", got)
	}
}

// TestSaveFromShortcut_OpensTheDialogOnlyWithChanges: Ctrl+S opens the Save
// dialog when the Draft moved since its last Commit, nothing otherwise
// (task 72_7; ADR 0021).
func TestSaveFromShortcut_OpensTheDialogOnlyWithChanges(t *testing.T) {
	f, _ := selected(t)
	f.run()
	if !f.l.hasUnsavedChanges() {
		t.Fatal("a Draft written but never saved reports no unsaved changes")
	}
	f.l.saveFromShortcut(f.l.compoCtx)
	if !f.l.saveDialog {
		t.Fatal("Ctrl+S did not open the Save dialog with unsaved changes")
	}
	saveWithUI(t, f, "First save")

	f.l.saveFromShortcut(f.l.compoCtx)
	if f.l.saveDialog {
		t.Error("Ctrl+S opened the Save dialog with nothing to save")
	}
}

// TestSaveCommit_ConflictRereadsTheDraft: another tab edits the Draft before
// this one saves: the 409 is a toast, and the Draft, the stack and the history
// are read again so the next Save states the server's Version (task 72_7).
func TestSaveCommit_ConflictRereadsTheDraft(t *testing.T) {
	f, id := selected(t)
	f.run()
	wt := f.widgetType(id)
	if _, err := f.api.UpdateWidgetType(context.Background(), id, apiv0.UpdateWidgetTypeRequest{
		Name: "Theirs", HtmlTemplate: wt.HtmlTemplate, Script: wt.Script, ScriptLanguage: wt.ScriptLanguage,
		DefaultWidth: wt.DefaultWidth, DefaultHeight: wt.DefaultHeight, InputPorts: wt.InputPorts, Version: wt.Version,
	}); err != nil {
		t.Fatalf("update widget type: %v", err)
	}

	f.l.saveAuthor, f.l.saveTitle = "Ada", "Stale save"
	f.l.saveCommit()
	f.run()
	if got := f.oneToast(); got.Status != http.StatusConflict {
		t.Fatalf("the toast %+v: want the server's 409", got)
	}
	if got, want := f.l.draftVersion(), f.widgetType(id).Version; got != want {
		t.Errorf("the known Version %d, the server's %d: the conflict did not reread the Draft", got, want)
	}

	// The next Save starts from the Version the conflict left.
	f.l.saveAuthor, f.l.saveTitle = "Ada", "After the conflict"
	f.l.saveCommit()
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "saved") {
		t.Fatalf("the second Save: %+v", got)
	}
}

// TestLoadSavedState_ResetsToTheLastSavedStateAndClearsTheIndicator: loading
// the last saved state drops the unsaved changes, puts the Draft back to it and
// turns the indicator off even though the Draft's Version rose (task 72_7;
// task 84; ADR 0021).
func TestLoadSavedState_ResetsToTheLastSavedStateAndClearsTheIndicator(t *testing.T) {
	f, kept := selected(t)
	f.run()
	saveWithUI(t, f, "First save")

	added := createWidgetType(t, f.api, "Added")
	f.event(apiv0.Event{Type: apiv0.EventWidgetTypeCreated, ID: added.String(), LibraryID: f.l.libraryID.String()})
	if !f.l.hasUnsavedChanges() {
		t.Fatal("the change after the saved state is not unsaved")
	}

	f.l.doLoadSavedState(f.l.compoCtx, 1)
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Saved state 1 loaded") {
		t.Errorf("the load confirmation: %+v", got)
	}
	if f.l.hasUnsavedChanges() {
		t.Error("the Draft still reports unsaved changes after loading its last saved state")
	}
	list, err := f.api.ListWidgetTypes(context.Background())
	if err != nil {
		t.Fatalf("server widget types: %v", err)
	}
	if len(list) != 1 || list[0].ID != kept {
		t.Errorf("the server's Draft after the load: %+v, want only the saved one", list)
	}
	if len(f.l.shownTypes()) != 1 || f.l.shownTypes()[0].ID != kept {
		t.Errorf("the shown Draft after the load: %+v", f.l.shownTypes())
	}
}

// TestLoadSavedState_ReplacesTheEditor: a text typed into the editor but never
// applied does not survive the load of a saved state — the editor takes the
// loaded WidgetType as it is, instead of merging the old typing into it
// (task 84).
func TestLoadSavedState_ReplacesTheEditor(t *testing.T) {
	f, id := selected(t)
	f.run()
	saveWithUI(t, f, "First save")

	f.l.selectItem(id)
	f.l.edit.script = "typed but never applied"
	f.run()

	f.l.doLoadSavedState(f.l.compoCtx, 1)
	f.run()
	f.e.TakeToasts() // the load's own confirmation

	if f.l.edit.script == "typed but never applied" {
		t.Errorf("the load kept the typed text: %q", f.l.edit.script)
	}
	if want := f.widgetType(id).Script; f.l.edit.script != want {
		t.Errorf("the editor holds %q after the load, want the saved %q", f.l.edit.script, want)
	}
}

// TestLoadSavedState_OlderSavedStateLeavesTheDraftUnsaved: a saved state that
// is not the last one becomes the Draft too, and the server then reports it
// unsaved — it differs from the newest saved state (task 74; task 84).
func TestLoadSavedState_OlderSavedStateLeavesTheDraftUnsaved(t *testing.T) {
	f, _ := selected(t)
	f.run()
	saveWithUI(t, f, "First save")

	second := createWidgetType(t, f.api, "Second")
	f.event(apiv0.Event{Type: apiv0.EventWidgetTypeCreated, ID: second.String(), LibraryID: f.l.libraryID.String()})
	f.run()
	saveWithUI(t, f, "Second save")

	f.l.doLoadSavedState(f.l.compoCtx, 1)
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Saved state 1 loaded") {
		t.Errorf("the load confirmation: %+v", got)
	}
	if !f.l.hasUnsavedChanges() {
		t.Error("the Draft loaded from an older saved state reports no unsaved changes")
	}
	if len(f.l.shownTypes()) != 1 {
		t.Errorf("the shown Draft after loading saved state 1: %+v, want its one WidgetType", f.l.shownTypes())
	}
}

// TestLibraryState_ListsTheStates: the header's state of the selected library
// follows the Draft: never saved, a saved state, unsaved changes (task 72_7;
// task 73; task 85_7).
func TestLibraryState_ListsTheStates(t *testing.T) {
	f, _ := selected(t)
	f.run()
	status := func() string { return app.HTMLString(f.l.renderLibraryStatus()) }
	if html := status(); !strings.Contains(html, "Never saved.") {
		t.Errorf("the header of a Draft never saved: %s", html)
	}

	saveWithUI(t, f, "First save")
	f.run()
	if html := status(); !strings.Contains(html, "Saved state #1") {
		t.Errorf("the header does not name the saved state: %s", html)
	}

	added := createWidgetType(t, f.api, "Added")
	f.event(apiv0.Event{Type: apiv0.EventWidgetTypeCreated, ID: added.String(), LibraryID: f.l.libraryID.String()})
	if html := status(); !strings.Contains(html, "unsaved changes") {
		t.Errorf("the header does not mark the unsaved Draft: %s", html)
	}
}

// TestServerEvents_AnotherLibraryLeavesTheSelectedAlone: a Commit of another
// library is not the selected one's business (task 72_7).
func TestServerEvents_AnotherLibraryLeavesTheSelectedAlone(t *testing.T) {
	url, api := newServer(t)
	other, err := api.CreateLibrary(context.Background(), apiv0.CreateLibraryRequest{Name: "Other"})
	if err != nil {
		t.Fatalf("create the other library: %v", err)
	}
	f := mount(t, url, api)
	f.run()
	f.l.commits = []apiv0.SavedStateResponse{{Number: 99}}

	for _, ev := range []apiv0.Event{
		{Type: apiv0.EventSavedStateCreated, ID: other.ID.String(), SavedState: 7},
	} {
		f.event(ev)
		if len(f.l.commits) != 1 || f.l.commits[0].Number != 99 {
			t.Fatalf("%s of another library reloaded the history: %+v", ev.Type, f.l.commits)
		}
	}
}

// TestServerEvents_ProjectResyncLeavesTheLibraryAlone: the Bar's stream may
// follow a Project while Library is shown; a resync of that stream makes none
// of the Library's lists stale (task 72_7).
func TestServerEvents_ProjectResyncLeavesTheLibraryAlone(t *testing.T) {
	f, _ := selected(t)
	f.run()
	f.l.commits = []apiv0.SavedStateResponse{{Number: 99}}

	f.event(apiv0.Event{Type: eventlog.EventProjectResync})
	if len(f.l.commits) != 1 || f.l.commits[0].Number != 99 {
		t.Errorf("a project resync reloaded the history: %+v", f.l.commits)
	}
}

// TestDiscard_AnotherTabsDiscardClearsTheIndicator: a Discard performed
// elsewhere arrives as the Draft's content events; the indicator is the flag
// the server answers (task 74; ADR 0016, amendment 2026-11-10).
func TestDiscard_AnotherTabsDiscardClearsTheIndicator(t *testing.T) {
	f, _ := selected(t)
	f.run()
	saveWithUI(t, f, "First save")

	createWidgetType(t, f.api, "Added")
	f.l.loadUnsavedChanges(f.l.compoCtx)
	f.run()
	if !f.l.hasUnsavedChanges() {
		t.Fatal("the change after the Commit is not unsaved")
	}

	if _, err := f.api.Discard(context.Background()); err != nil {
		t.Fatalf("another tab's Discard: %v", err)
	}
	// The events of the Discard reach this tab: the WidgetType it removed is
	// one.
	f.event(apiv0.Event{
		Type: apiv0.EventWidgetTypeDeleted, ID: uuid.NewString(), LibraryID: f.l.libraryID.String(),
	})
	if f.l.hasUnsavedChanges() {
		t.Errorf("the indicator stays after another tab's Discard")
	}
	if html := app.HTMLString(app.Div().Body(f.l.renderCommitBar()...)); strings.Contains(html, "data-commit-unsaved") {
		t.Errorf("the header still marks unsaved changes: %s", html)
	}
}
