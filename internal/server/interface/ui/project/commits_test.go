package project

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestSaveCommit_HistoryRowAndIndicator: Save fixes the Draft as Commit 1, the
// history shows it and the unsaved-changes indicator goes off (task 72_2;
// ADR 0021).
func TestSaveCommit_HistoryRowAndIndicator(t *testing.T) {
	f := startProject(t)
	f.run()
	if !f.p.commitsLoaded {
		t.Fatal("the history was not loaded")
	}
	if !f.p.hasUnsavedChanges() {
		t.Fatalf("a fresh Draft reports no unsaved changes against no Commit")
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-commit-unsaved`) {
		t.Errorf("the header does not mark unsaved changes: %s", html)
	}

	f.p.openSaveDialog(f.p.compoCtx)
	f.p.saveAuthor = "Ada"
	f.p.saveTitle = "First save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()

	f.e.TakeToasts() // the Save's own confirmation
	if len(f.p.commits) != 1 || f.p.commits[0].Number != 1 || f.p.commits[0].Author != "Ada" {
		t.Fatalf("the history: %+v", f.p.commits)
	}
	if f.p.hasUnsavedChanges() {
		t.Errorf("the Draft still reports unsaved changes after a Save")
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-commit-saved`) {
		t.Errorf("the header does not mark the Draft saved: %s", html)
	}

	f.p.openCommitsDialog(f.p.compoCtx)
	f.run()
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-commit-row="1"`) {
		t.Errorf("the history does not show Commit 1: %s", html)
	}
	// The content of a Commit is read by its own button (task 72_2).
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-commit-download="1"`) {
		t.Errorf("the history offers no download of Commit 1: %s", html)
	}
}

// TestSaveCommit_SuccessAndNothingToSave: a Save shows its toast, the shortcut
// opens the dialog only while the Draft has changes, and a Save of an unchanged
// Draft is the server's 409 (task 72_6; ADR 0021).
func TestSaveCommit_SuccessAndNothingToSave(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"
	f.p.saveTitle = "First save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Commit 1 saved") {
		t.Errorf("the Save showed %+v, want its confirmation", got)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-commit-saved`) {
		t.Errorf("the header does not mark the Draft saved: %s", html)
	}

	// Nothing moved since the Commit: Ctrl+S does nothing and the Save button
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
	f.p.saveCommit(f.p.compoCtx)
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
	f.p.saveCommit(f.p.compoCtx)
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

// TestAuthorBar_ShowsTheStoredName: the header names the author the Save and
// Release dialogs fill in, with a button to change it — not a lock (task 72_6).
func TestAuthorBar_ShowsTheStoredName(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"

	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Author: Ada") || !strings.Contains(html, `data-author-name="Ada"`) {
		t.Errorf("the header does not name the author: %s", html)
	}
	if !strings.Contains(html, `data-author-change`) {
		t.Errorf("the header offers no way to change the author: %s", html)
	}
}

// TestSaveCommit_ShortcutsWaitForTheDialogs: the Save and the History dialogs
// of the mode hold the Draft's shortcuts, like its other dialogs (task 72_2;
// ADR 0021).
func TestSaveCommit_ShortcutsWaitForTheDialogs(t *testing.T) {
	f := startProject(t)
	f.run()
	if !f.p.keyActionAllowed() {
		t.Fatal("a shortcut is refused without a dialog")
	}
	f.p.saveDialog = true
	if f.p.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the Save dialog")
	}
	f.p.saveDialog, f.p.commitsDialog = false, true
	if f.p.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the History dialog")
	}
}

// TestSaveCommit_ConflictRereadsAndRename: a Save of a stale Version is a
// conflict that rereads the Draft, and a Commit's title is renamed in place
// (task 72_2; ADR 0021).
func TestSaveCommit_ConflictRereadsAndRename(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"
	f.p.saveTitle = "First save"
	f.p.saveCommit(f.p.compoCtx)
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
	f.p.saveCommit(f.p.compoCtx)
	f.run()

	if got := f.oneToast(); got.Status != 409 {
		t.Fatalf("a stale Save: toast %+v, want a 409", got)
	}
	if f.p.draftVersion() <= f.p.lastCommitVersion() {
		t.Errorf("the conflict did not reread the Draft: version %d, last commit %d",
			f.p.draftVersion(), f.p.lastCommitVersion())
	}
	f.p.saveDialog = false

	f.p.startRenamingCommit(1, "First save")
	f.p.renameCommitTitle = "Before the demo"
	f.p.renameCommit(f.p.compoCtx)
	f.run()
	f.noToasts()
	if len(f.p.commits) != 1 || f.p.commits[0].Title != "Before the demo" {
		t.Errorf("the renamed history: %+v", f.p.commits)
	}
	if f.p.renamingCommit != 0 {
		t.Errorf("the inline rename editor stayed open")
	}
}
