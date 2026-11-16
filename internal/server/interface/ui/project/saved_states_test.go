package project

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/username"
)

// TestSavedStatesButton_TogglesTheContent: the panel's list opens the saved
// states and pressing it again returns to the content (task 91).
func TestSavedStatesButton_TogglesTheContent(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.toggleHistory(f.p.compoCtx)
	f.run()
	if !f.p.historyScreen {
		t.Fatal("the saved-states button did not open the history")
	}
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `aria-label="Back to the Draft"`) {
		t.Errorf("the panel does not offer the way back while the history is open: %s", html)
	}
	if !strings.Contains(html, `aria-label="Close the saved states"`) {
		t.Errorf("the list does not say it closes the history: %s", html)
	}
	f.p.toggleHistory(f.p.compoCtx)
	f.run()
	if f.p.historyScreen {
		t.Error("the saved-states button did not return to the content")
	}
}

// TestSaveSavedState_HistoryRowAndIndicator: Save fixes the Draft as saved
// state 1, the history shows it and the unsaved-changes indicator goes off
// (task 72_2; ADR 0021).
func TestSaveSavedState_HistoryRowAndIndicator(t *testing.T) {
	f := startProject(t)
	f.run()
	if !f.p.savedStatesLoaded {
		t.Fatal("the history was not loaded")
	}
	// A Draft nothing was written to has no unsaved changes and no badge
	// (task 74; ADR 0016, amendment 2026-11-10).
	if f.p.hasUnsavedChanges() {
		t.Fatalf("a Draft nothing was written to reports unsaved changes")
	}
	f.p.createScene(f.p.compoCtx)
	f.run()
	if !f.p.hasUnsavedChanges() {
		t.Fatalf("the change after the saved state is not unsaved")
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-saved-state-unsaved`) {
		t.Errorf("the header does not mark unsaved changes: %s", html)
	}

	f.p.openHistory(f.p.compoCtx)
	f.p.saveAuthor = "Ada"
	f.run()
	html := app.HTMLString(f.p.Render())
	if !f.p.historyScreen || !strings.Contains(html, `data-save-title`) {
		t.Errorf("the Save button did not land on the history form: %s", html)
	}
	if strings.Contains(html, `data-save-dialog`) {
		t.Errorf("the Save dialog still exists: %s", html)
	}
	f.p.saveSavedState(f.p.compoCtx, "First save")
	f.run()

	f.e.TakeToasts() // the Save's own confirmation
	if len(f.p.savedStates) != 1 || f.p.savedStates[0].Number != 1 || f.p.savedStates[0].Author != "Ada" {
		t.Fatalf("the history: %+v", f.p.savedStates)
	}
	if f.p.hasUnsavedChanges() {
		t.Errorf("the Draft still reports unsaved changes after a Save")
	}
	html = app.HTMLString(f.p.Render())
	for _, unwanted := range []string{`data-saved-state-saved`, "Saved state #", "data-deploy-running"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("the panel still names the saved state (%s): %s", unwanted, html)
		}
	}

	f.p.openHistory(f.p.compoCtx)
	f.run()
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-history-screen`) || !strings.Contains(html, `data-saved-state-row="1"`) {
		t.Errorf("the history does not show saved state 1: %s", html)
	}
	// The row's action is the load of the saved state: the Project mode knows
	// no Release (task 85_5).
	if !strings.Contains(html, `data-icon="load"`) {
		t.Errorf("the row offers no load: %s", html)
	}
	if strings.Contains(html, `>Make Release<`) {
		t.Errorf("the row still offers a Release: %s", html)
	}
}

// TestSaveSavedState_SuccessAndNothingToSave: a Save shows its toast, the
// shortcut opens the dialog only while the Draft has changes, and a Save of an
// unchanged Draft is the server's 409 (task 72_6; ADR 0021).
func TestSaveSavedState_SuccessAndNothingToSave(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"
	f.p.saveSavedState(f.p.compoCtx, "First save")
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Saved state 1 saved") {
		t.Errorf("the Save showed %+v, want its confirmation", got)
	}
	if html := app.HTMLString(f.p.Render()); strings.Contains(html, `data-saved-state-saved`) {
		t.Errorf("the panel still marks a gray saved state: %s", html)
	}

	// Nothing moved since the SavedState: Ctrl+S does nothing and the Draft
	// row's Save is off, saying why.
	f.p.closeHistory()
	f.p.saveFromShortcut(f.p.compoCtx)
	f.run()
	if f.p.historyScreen {
		t.Errorf("Ctrl+S opened the history screen with nothing to save")
	}
	f.p.openHistory(f.p.compoCtx)
	f.run()
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Nothing to save since the last save") {
		t.Errorf("the Draft row's Save does not say there is nothing to save: %s", html)
	}

	// A Save forced anyway is the server's refusal.
	f.p.openHistory(f.p.compoCtx)
	f.p.saveSavedState(f.p.compoCtx, "Unchanged")
	f.run()
	if got := f.oneToast(); got.Status != http.StatusConflict {
		t.Errorf("a Save of an unchanged Draft: toast %+v, want a 409", got)
	}
}

// TestSaveShortcut_WithChangesOpensTheHistory: Ctrl+S opens the history with
// its Save form once the Draft has changes (task 72_6, task 91).
func TestSaveShortcut_WithChangesOpensTheHistory(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"
	f.p.saveSavedState(f.p.compoCtx, "First save")
	f.run()
	f.e.TakeToasts() // the Save's own confirmation
	f.p.closeHistory()

	f.p.createScene(f.p.compoCtx)
	f.run()
	f.noToasts()
	f.p.saveFromShortcut(f.p.compoCtx)
	f.run()
	if !f.p.historyScreen {
		t.Errorf("Ctrl+S did not open the history screen with unsaved changes")
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-save-title`) {
		t.Errorf("the history screen has no Save form: %s", html)
	}
}

// TestHeader_HasNoAuthorBar: the user name is set in the Misc section and is
// not repeated in the header; the Save form at the top of the history takes it
// as the author, and the User name page's change reaches it (task 90, task 91).
func TestHeader_HasNoAuthorBar(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"

	html := app.HTMLString(f.p.Render())
	if strings.Contains(html, `data-author-bar`) || strings.Contains(html, "Author: Ada") {
		t.Errorf("the header still shows the author: %s", html)
	}

	f.p.compoCtx.Dispatch(func(ctx app.Context) {
		ctx.NewActionWithValue(username.ActionChanged, "Grace")
	})
	f.run()
	if f.p.saveAuthor != "Grace" {
		t.Errorf("the author after the User name page wrote one: %q", f.p.saveAuthor)
	}
	f.p.openHistory(f.p.compoCtx)
	f.run()
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-save-title`) {
		t.Errorf("the Save form has no name field: %s", html)
	}
	if strings.Contains(html, `data-save-author`) {
		t.Errorf("the Save form still asks for an author: %s", html)
	}
}

// TestSaveSavedState_ShortcutsWaitForTheDialogs: the History and Deploy screens
// of the mode hold the Draft's shortcuts, like its other dialogs (task 72_2,
// task 85_5, task 91; ADR 0021).
func TestSaveSavedState_ShortcutsWaitForTheDialogs(t *testing.T) {
	f := startProject(t)
	f.run()
	if !f.p.keyActionAllowed() {
		t.Fatal("a shortcut is refused without a dialog")
	}
	f.p.historyScreen = true
	if f.p.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the History screen")
	}
	f.p.historyScreen, f.p.deployDialog = false, true
	if f.p.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the Deploy window")
	}
}

// TestSaveSavedState_ConflictRereadsAndRename: a Save of a stale Version is a
// conflict that rereads the Draft, and a saved state's title is renamed in
// place (task 72_2; ADR 0021).
func TestSaveSavedState_ConflictRereadsAndRename(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"
	f.p.saveSavedState(f.p.compoCtx, "First save")
	f.run()
	f.e.TakeToasts() // the Save's own confirmation

	// Another tab writes the Draft: the Version rises above the one this tab
	// saved at.
	if _, err := f.api.CreateTag(context.Background(), apiv0.CreateTagRequest{Name: "speed", Type: "integer"}); err != nil {
		t.Fatalf("another tab's write: %v", err)
	}
	// This tab still states the Version it knew: forgetting it makes the next
	// Save state 0, which the server refuses as stale.
	f.p.forgetDraftVersion()
	f.p.saveSavedState(f.p.compoCtx, "Stale save")
	f.run()

	if got := f.oneToast(); got.Status != 409 {
		t.Fatalf("a stale Save: toast %+v, want a 409", got)
	}
	if len(f.p.savedStates) == 0 || f.p.draftVersion() <= f.p.savedStates[0].DraftVersion {
		t.Errorf("the conflict did not reread the Draft: version %d, last saved state %d",
			f.p.draftVersion(), f.p.savedStates[0].DraftVersion)
	}
	f.p.closeHistory()

	f.p.renameSavedState(f.p.compoCtx, 1, "Before the demo")
	f.run()
	f.noToasts()
	if len(f.p.savedStates) != 1 || f.p.savedStates[0].Title != "Before the demo" {
		t.Errorf("the renamed history: %+v", f.p.savedStates)
	}
}

// TestOpenHistory_StartsWithAFreshName: the name of the saved state being made
// lives in the history screen alone, so a name typed for one project never
// reaches the next history screen — it opens on a fresh one (task 91).
func TestOpenHistory_StartsWithAFreshName(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.openHistory(f.p.compoCtx)
	f.p.history.StartNew("Typed for this project")
	f.p.closeHistory()
	f.p.openHistory(f.p.compoCtx)
	f.run()

	html := app.HTMLString(f.p.Render())
	if strings.Contains(html, "Typed for this project") {
		t.Errorf("the history reopened with the name of the project before: %s", html)
	}
	if !strings.Contains(html, `data-save-title`) {
		t.Errorf("the history reopened without its Save form: %s", html)
	}
}
