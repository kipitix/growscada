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

	f.p.openSaveDialog(f.p.compoCtx)
	f.p.saveAuthor = "Ada"
	f.p.saveTitle = "First save"
	f.p.saveSavedState(f.p.compoCtx)
	f.run()

	f.e.TakeToasts() // the Save's own confirmation
	if len(f.p.savedStates) != 1 || f.p.savedStates[0].Number != 1 || f.p.savedStates[0].Author != "Ada" {
		t.Fatalf("the history: %+v", f.p.savedStates)
	}
	if f.p.hasUnsavedChanges() {
		t.Errorf("the Draft still reports unsaved changes after a Save")
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-saved-state-saved`) {
		t.Errorf("the header does not mark the Draft saved: %s", html)
	}

	f.p.openHistory(f.p.compoCtx)
	f.run()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-history-screen`) || !strings.Contains(html, `data-saved-state-row="1"`) {
		t.Errorf("the history does not show saved state 1: %s", html)
	}
	// The row's action is the load of the saved state: the Project mode knows
	// no Release (task 85_5).
	if !strings.Contains(html, `>Load<`) {
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
	f.p.saveTitle = "First save"
	f.p.saveSavedState(f.p.compoCtx)
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Saved state 1 saved") {
		t.Errorf("the Save showed %+v, want its confirmation", got)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-saved-state-saved`) {
		t.Errorf("the header does not mark the Draft saved: %s", html)
	}

	// Nothing moved since the SavedState: Ctrl+S does nothing and the Save button
	// says why it is off.
	f.p.saveDialog = false
	f.p.saveFromShortcut(f.p.compoCtx)
	f.run()
	if f.p.saveDialog {
		t.Errorf("Ctrl+S opened the Save dialog with nothing to save")
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Nothing to save since the last save") {
		t.Errorf("the Save button does not say there is nothing to save: %s", html)
	}

	// A Save forced anyway is the server's refusal.
	f.p.openSaveDialog(f.p.compoCtx)
	f.p.saveSavedState(f.p.compoCtx)
	f.run()
	if got := f.oneToast(); got.Status != http.StatusConflict {
		t.Errorf("a Save of an unchanged Draft: toast %+v, want a 409", got)
	}
}

// TestSaveShortcut_WithChangesOpensTheDialog: Ctrl+S asks for the title and the
// author as the Save button does, once the Draft has changes (task 72_6).
func TestSaveShortcut_WithChangesOpensTheDialog(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"
	f.p.saveTitle = "First save"
	f.p.saveSavedState(f.p.compoCtx)
	f.run()
	f.e.TakeToasts() // the Save's own confirmation
	f.p.saveDialog = false

	f.p.createScene(f.p.compoCtx)
	f.run()
	f.noToasts()
	f.p.saveFromShortcut(f.p.compoCtx)
	f.run()
	if !f.p.saveDialog {
		t.Errorf("Ctrl+S did not open the Save dialog with unsaved changes")
	}
}

// TestHeader_HasNoAuthorBar: the user name is set in the Misc section and is
// not repeated in the header; the Save dialog still fills it in, and the
// User name page's change reaches the next dialog (task 90).
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
	f.p.openSaveDialog(f.p.compoCtx)
	f.run()
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-save-author`) ||
		!strings.Contains(html, `value="Grace"`) {
		t.Errorf("the Save dialog does not fill in the user name: %s", html)
	}
}

// TestSaveSavedState_ShortcutsWaitForTheDialogs: the Save, History and Deploy
// dialogs of the mode hold the Draft's shortcuts, like its other dialogs
// (task 72_2, task 85_5; ADR 0021).
func TestSaveSavedState_ShortcutsWaitForTheDialogs(t *testing.T) {
	f := startProject(t)
	f.run()
	if !f.p.keyActionAllowed() {
		t.Fatal("a shortcut is refused without a dialog")
	}
	f.p.saveDialog = true
	if f.p.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the Save dialog")
	}
	f.p.saveDialog, f.p.historyScreen = false, true
	if f.p.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the History dialog")
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
	f.p.saveTitle = "First save"
	f.p.saveSavedState(f.p.compoCtx)
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
	f.p.saveTitle = "Stale save"
	f.p.saveSavedState(f.p.compoCtx)
	f.run()

	if got := f.oneToast(); got.Status != 409 {
		t.Fatalf("a stale Save: toast %+v, want a 409", got)
	}
	if len(f.p.savedStates) == 0 || f.p.draftVersion() <= f.p.savedStates[0].DraftVersion {
		t.Errorf("the conflict did not reread the Draft: version %d, last saved state %d",
			f.p.draftVersion(), f.p.savedStates[0].DraftVersion)
	}
	f.p.saveDialog = false

	f.p.renameSavedState(f.p.compoCtx, 1, "Before the demo")
	f.run()
	f.noToasts()
	if len(f.p.savedStates) != 1 || f.p.savedStates[0].Title != "Before the demo" {
		t.Errorf("the renamed history: %+v", f.p.savedStates)
	}
}
