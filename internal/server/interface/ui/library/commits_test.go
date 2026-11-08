package library

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// TestSaveCommit_LibraryHistoryAndRename: Save fixes the Draft as Commit 1,
// the indicator goes off, the history shows it and its title is renamed in
// place (task 72_2; ADR 0021).
func TestSaveCommit_LibraryHistoryAndRename(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.run()
	if !f.l.commitsLoaded || f.l.libraryID != api.LibraryClient.ID() {
		t.Fatalf("the library was not selected or its history not loaded")
	}
	if !f.l.hasUnsavedChanges() {
		t.Fatalf("a fresh Draft reports no unsaved changes against no Commit")
	}
	if html := app.HTMLString(f.l.Render()); !strings.Contains(html, `data-commit-unsaved`) {
		t.Errorf("the column does not mark unsaved changes: %s", html)
	}

	f.l.saveAuthor = "Ada"
	f.l.saveTitle = "Widgets before the demo"
	f.l.saveCommit()
	f.run()

	f.noToasts()
	if len(f.l.commits) != 1 || f.l.commits[0].Number != 1 || f.l.commits[0].Author != "Ada" {
		t.Fatalf("the history: %+v", f.l.commits)
	}
	if f.l.hasUnsavedChanges() {
		t.Errorf("the Draft still reports unsaved changes after a Save")
	}

	f.l.openCommitsDialog()
	f.run()
	if html := app.HTMLString(f.l.Render()); !strings.Contains(html, `data-commit-row="1"`) {
		t.Errorf("the history does not show Commit 1: %s", html)
	}

	f.l.renamingCommit, f.l.renameCommitTitle = 1, "For the demo"
	f.l.renameCommit()
	f.run()
	f.noToasts()
	if len(f.l.commits) != 1 || f.l.commits[0].Title != "For the demo" {
		t.Errorf("the renamed history: %+v", f.l.commits)
	}
}

// TestSaveCommit_LibraryShortcutsWaitForTheDialogs: the Save and the History
// dialogs of the mode hold the Draft's shortcuts (task 72_2; ADR 0021).
func TestSaveCommit_LibraryShortcutsWaitForTheDialogs(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.run()
	if !f.l.keyActionAllowed() {
		t.Fatal("a shortcut is refused without a dialog")
	}
	f.l.saveDialog = true
	if f.l.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the Save dialog")
	}
	f.l.saveDialog, f.l.commitsDialog = false, true
	if f.l.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the History dialog")
	}
}
