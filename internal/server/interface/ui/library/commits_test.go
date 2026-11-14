package library

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestCommitBar_IndicatorKeepsTheButtonPositions: the unsaved-changes indicator
// is always the panel's second child — hidden when it has nothing to say —, so
// History never shifts by a position and a reused element keeps its own handler
// (task 79). TestEngine renders but dispatches no DOM click, so
// the panels themselves are checked in the browser; this test guards the
// structure the bug came from.
func TestCommitBar_IndicatorKeepsTheButtonPositions(t *testing.T) {
	l := &Library{libraryID: uuid.New()}
	for _, tc := range []struct {
		name    string
		unsaved bool
		marker  string
	}{
		{"unsaved changes", true, `data-commit-unsaved`},
		{"nothing to show", false, `display:none`},
	} {
		l.draftUnsaved, l.draftStateLoaded = tc.unsaved, false
		bar := l.renderCommitBar()
		if len(bar) != 3 {
			t.Fatalf("%s: the header has %d children, want 3", tc.name, len(bar))
		}
		if html := app.HTMLString(bar[0]); !strings.Contains(html, `data-icon="save"`) {
			t.Errorf("%s: the first child is not Save: %s", tc.name, html)
		}
		if html := app.HTMLString(bar[1]); !strings.Contains(html, tc.marker) {
			t.Errorf("%s: the indicator: %s", tc.name, html)
		}
		if html := app.HTMLString(bar[2]); !strings.Contains(html, `data-icon="history"`) {
			t.Errorf("%s: the third child is not History: %s", tc.name, html)
		}

	}
}

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

	if got := f.oneToast(); !strings.Contains(got.Title, "Saved state 1 saved") {
		t.Errorf("the Save confirmation: %+v", got)
	}
	if len(f.l.commits) != 1 || f.l.commits[0].Number != 1 || f.l.commits[0].Author != "Ada" {
		t.Fatalf("the history: %+v", f.l.commits)
	}
	if f.l.hasUnsavedChanges() {
		t.Errorf("the Draft still reports unsaved changes after a Save")
	}

	f.l.openHistory()
	f.run()
	if html := app.HTMLString(f.l.Render()); !strings.Contains(html, `data-saved-state-row="1"`) {
		t.Errorf("the history does not show Commit 1: %s", html)
	}

	f.l.renameSavedState(f.l.compoCtx, 1, "For the demo")
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

	html := app.HTMLString(f.l.renderHistoryScreen())
	if !strings.Contains(html, "Make Release") || strings.Contains(html, ">Released<") {
		t.Errorf("the saved state row before the release: %s", html)
	}

	releaseNow(t, f)
	f.run()
	html = app.HTMLString(f.l.renderHistoryScreen())
	if !strings.Contains(html, "Released") {
		t.Errorf("the saved state row after the release: %s", html)
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
	f.l.saveDialog, f.l.historyScreen = false, true
	if f.l.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the History dialog")
	}
}
