package library

import (
	"context"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestSaveCommit_LibraryHistoryAndRename: Save fixes the Draft as Commit 1,
// the indicator goes off, the history shows it and its title is renamed in
// place (task 72_2; ADR 0021).
func TestSaveCommit_LibraryHistoryAndRename(t *testing.T) {
	url, api := newServer(t)
	// A library with a change to save: a Draft nothing was written to is clean
	// and has nothing to fix as a Commit (task 74).
	createWidgetType(t, api, "Gauge")
	f := mount(t, url, api)
	f.run()
	if !f.l.commitsLoaded || f.l.libraryID != api.LibraryClient.ID() {
		t.Fatalf("the library was not selected or its history not loaded")
	}
	if !f.l.hasUnsavedChanges() {
		t.Fatalf("a Draft written but never saved reports no unsaved changes")
	}
	if html := app.HTMLString(f.l.Render()); !strings.Contains(html, `data-commit-unsaved`) {
		t.Errorf("the column does not mark unsaved changes: %s", html)
	}

	f.l.saveAuthor = "Ada"
	f.l.saveTitle = "Widgets before the demo"
	f.l.saveCommit()
	f.run()

	if got := f.oneToast(); !strings.Contains(got.Title, "Commit 1 saved") {
		t.Errorf("the Save confirmation: %+v", got)
	}
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

// TestCommitRow_ReleaseButton: a Commit of the history offers "Make Release";
// once a Release publishes it, the button says so (task 72_5; ADR 0021).
func TestCommitRow_ReleaseButton(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.run()
	f.l.createItem(f.l.compoCtx)
	f.run()

	state, err := api.DraftState(context.Background())
	if err != nil {
		t.Fatalf("DraftState: %v", err)
	}
	if _, err := api.SaveCommit(context.Background(), apiv0.CreateCommitRequest{Version: state.Version, Title: "Save", Author: "Ada"}); err != nil {
		t.Fatalf("SaveCommit: %v", err)
	}
	f.l.loadCommits(f.l.compoCtx)
	f.run()

	html := app.HTMLString(f.l.renderCommitRow(f.l.commits[0]))
	if !strings.Contains(html, "Make Release") || strings.Contains(html, ">Released<") {
		t.Errorf("the commit row before the release: %s", html)
	}

	releaseNow(t, f)
	f.run()
	html = app.HTMLString(f.l.renderCommitRow(f.l.commits[0]))
	if !strings.Contains(html, "Released") {
		t.Errorf("the commit row after the release: %s", html)
	}
}

// TestReleaseHistoryLabel_CommitAndLegacy: a Release names the Commit it
// published, or says it predates Commits (task 72_5).
func TestReleaseHistoryLabel_CommitAndLegacy(t *testing.T) {
	number, title := 3, "Widgets of the release"
	got := releaseHistoryLabel(apiv0.LibraryReleaseResponse{
		Number: 2, CommitNumber: &number, Title: &title, Note: "a note", Origin: apiv0.LibraryReleaseOriginCommit,
	})
	if want := "Release 2 — commit #3 Widgets of the release — a note"; got != want {
		t.Errorf("the label: %q, want %q", got, want)
	}
	if got := releaseHistoryLabel(apiv0.LibraryReleaseResponse{Number: 1, Origin: apiv0.LibraryReleaseOriginLegacy}); got != "Release 1 — legacy" {
		t.Errorf("the legacy label: %q", got)
	}
}

// TestReleaseNoteEvent_ReloadsTheHistory: a note changed in another tab
// (library_release_note_updated) rereads the Releases of the selected library
// (task 72_5; ADR 0021).
func TestReleaseNoteEvent_ReloadsTheHistory(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.run()
	f.l.createItem(f.l.compoCtx)
	f.run()
	releaseNow(t, f)
	if len(f.l.releases) != 1 || f.l.releases[0].Note != "" {
		t.Fatalf("the releases before the event: %+v", f.l.releases)
	}

	if _, err := f.api.UpdateReleaseNote(context.Background(), 1, apiv0.UpdateLibraryReleaseNoteRequest{Note: "renoted"}); err != nil {
		t.Fatalf("UpdateReleaseNote: %v", err)
	}
	f.event(apiv0.Event{Type: apiv0.EventLibraryReleaseNoteUpdated, ID: f.l.libraryID.String(), Release: 1})
	if len(f.l.releases) != 1 || f.l.releases[0].Note != "renoted" {
		t.Errorf("the releases after the event: %+v, want the renamed note", f.l.releases)
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
