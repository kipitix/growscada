package library

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/username"
)

// TestSavedStateBar_IndicatorKeepsTheButtonPositions: the unsaved-changes
// indicator is always the panel's second child — hidden when it has nothing
// to say —, so History never shifts by a position and a reused element keeps
// its own handler (task 79). TestEngine renders but dispatches no DOM click,
// so the panels themselves are checked in the browser; this test guards the
// structure the bug came from.
func TestSavedStateBar_IndicatorKeepsTheButtonPositions(t *testing.T) {
	l := &Library{libraryID: uuid.New()}
	for _, tc := range []struct {
		name    string
		unsaved bool
		marker  string
	}{
		{"unsaved changes", true, `data-saved-state-unsaved`},
		{"nothing to show", false, `display:none`},
	} {
		l.draftUnsaved, l.draftStateLoaded = tc.unsaved, false
		bar := l.renderSavedStateBar()
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

// TestSaveSavedState_LibraryHistoryAndRename: Save fixes the Draft as saved
// state 1, the indicator goes off, the history shows it and its title is
// renamed in place (task 72_2; ADR 0021).
func TestSaveSavedState_LibraryHistoryAndRename(t *testing.T) {
	url, api := newServer(t)
	// A library with a change to save: a Draft nothing was written to is clean
	// and has nothing to fix as a saved state (task 74).
	createWidgetType(t, api, "Gauge")
	f := mount(t, url, api)
	f.run()
	if !f.l.savedStatesLoaded || f.l.libraryID != api.LibraryClient.ID() {
		t.Fatalf("the library was not selected or its history not loaded")
	}
	if !f.l.hasUnsavedChanges() {
		t.Fatalf("a Draft written but never saved reports no unsaved changes")
	}
	if html := app.HTMLString(f.l.Render()); !strings.Contains(html, `data-saved-state-unsaved`) {
		t.Errorf("the column does not mark unsaved changes: %s", html)
	}

	f.l.saveAuthor = "Ada"
	f.l.saveTitle = "Widgets before the demo"
	f.l.saveSavedState()
	f.run()

	if got := f.oneToast(); !strings.Contains(got.Title, "Saved state 1 saved") {
		t.Errorf("the Save confirmation: %+v", got)
	}
	if len(f.l.savedStates) != 1 || f.l.savedStates[0].Number != 1 || f.l.savedStates[0].Author != "Ada" {
		t.Fatalf("the history: %+v", f.l.savedStates)
	}
	if f.l.hasUnsavedChanges() {
		t.Errorf("the Draft still reports unsaved changes after a Save")
	}

	f.l.openHistory()
	f.run()
	if html := app.HTMLString(f.l.Render()); !strings.Contains(html, `data-saved-state-row="1"`) {
		t.Errorf("the history does not show saved state 1: %s", html)
	}

	f.l.renameSavedState(f.l.compoCtx, 1, "For the demo")
	f.run()
	f.noToasts()
	if len(f.l.savedStates) != 1 || f.l.savedStates[0].Title != "For the demo" {
		t.Errorf("the renamed history: %+v", f.l.savedStates)
	}
}

// TestSaveSavedState_LibraryShortcutsWaitForTheDialogs: the Save and the
// History dialogs of the mode hold the Draft's shortcuts (task 72_2; ADR 0021).
func TestSaveSavedState_LibraryShortcutsWaitForTheDialogs(t *testing.T) {
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

// TestHeader_HasNoAuthorBar: the user name is set in the Misc section and is
// not repeated in the header; the Save dialog fills it in, and the User name
// page's change reaches the next dialog (task 90).
func TestHeader_HasNoAuthorBar(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.run()
	f.l.saveAuthor = "Ada"

	if html := app.HTMLString(f.l.Render()); strings.Contains(html, `data-author-bar`) ||
		strings.Contains(html, "Author: Ada") {
		t.Errorf("the header still shows the author: %s", html)
	}

	f.l.compoCtx.Dispatch(func(ctx app.Context) {
		ctx.NewActionWithValue(username.ActionChanged, "Grace")
	})
	f.run()
	if f.l.saveAuthor != "Grace" {
		t.Errorf("the author after the User name page wrote one: %q", f.l.saveAuthor)
	}
	f.l.openSaveDialog()
	f.run()
	if html := app.HTMLString(f.l.Render()); !strings.Contains(html, `data-save-author`) {
		t.Errorf("the Save dialog has no author field: %s", html)
	}
}
