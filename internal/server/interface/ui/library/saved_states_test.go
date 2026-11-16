package library

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/username"
)

// TestHeader_OneSavedStatesButtonAndTheUnsavedMark: the panel's only
// saved-states action is the list — it opens the history and stays enabled
// without changes —, the yellow unsaved mark follows Undo and Redo, and the
// panel names no saved state (task 91).
func TestHeader_OneSavedStatesButtonAndTheUnsavedMark(t *testing.T) {
	l := &Library{libraryID: uuid.New()}
	l.draftUnsaved = true
	html := app.HTMLString(l.renderHeader())
	if strings.Contains(html, `data-icon="history"`) {
		t.Errorf("the panel still has a History button: %s", html)
	}
	if got := strings.Count(html, `data-icon="saved-states"`); got != 1 {
		t.Errorf("the panel has %d saved-states buttons, want 1: %s", got, html)
	}
	if strings.Contains(html, `data-icon="save"`) {
		t.Errorf("the panel still carries the floppy, which belongs to Save: %s", html)
	}
	if i, j := strings.Index(html, "data-draft-changes-bar"), strings.Index(html, "data-saved-state-unsaved"); i < 0 || j < 0 || i > j {
		t.Errorf("the unsaved mark does not follow Undo and Redo: %s", html)
	}
	for _, unwanted := range []string{"data-saved-state-saved", "Saved state #", "data-library-status"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("the panel still names the saved state (%s): %s", unwanted, html)
		}
	}

	l.draftUnsaved = false
	if html := app.HTMLString(l.renderHeader()); strings.Contains(html, "data-saved-state-unsaved") {
		t.Errorf("a clean Draft still marks unsaved changes: %s", html)
	}
}

// TestSavedStatesButton_TogglesTheEditor: the panel's list opens the saved
// states and pressing it again returns to the Draft (task 91).
func TestSavedStatesButton_TogglesTheEditor(t *testing.T) {
	f, _ := selected(t)
	f.run()
	f.l.toggleHistory()
	f.run()
	if !f.l.historyScreen {
		t.Fatal("the saved-states button did not open the history")
	}
	html := app.HTMLString(f.l.renderHeader())
	if !strings.Contains(html, `aria-label="Back to the Draft"`) {
		t.Errorf("the panel does not offer the way back while the history is open: %s", html)
	}
	if !strings.Contains(html, `aria-label="Close the saved states"`) {
		t.Errorf("the list does not say it closes the history: %s", html)
	}
	f.l.toggleHistory()
	f.run()
	if f.l.historyScreen {
		t.Error("the saved-states button did not return to the editor")
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
	f.l.openHistory()
	if !f.l.historyScreen {
		t.Fatal("the Save button did not open the history screen")
	}
	if html := app.HTMLString(f.l.Render()); !strings.Contains(html, `data-save-title`) {
		t.Errorf("the history screen has no Save form: %s", html)
	}
	f.l.saveSavedState("Widgets before the demo")
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

// TestSaveSavedState_LibraryShortcutsWaitForTheHistory: the History screen of
// the mode holds the Draft's shortcuts (task 72_2, task 91; ADR 0021).
func TestSaveSavedState_LibraryShortcutsWaitForTheHistory(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)
	f.run()
	if !f.l.keyActionAllowed() {
		t.Fatal("a shortcut is refused without a dialog")
	}
	f.l.historyScreen = true
	if f.l.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the History screen")
	}
}

// TestHeader_HasNoAuthorBar: the user name is set in the Misc section and is
// not repeated in the header; the Save form at the top of the history takes it
// as the author, and the User name page's change reaches it (task 90, task 91).
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
	f.l.openHistory()
	f.run()
	html := app.HTMLString(f.l.Render())
	if !strings.Contains(html, `data-save-title`) {
		t.Errorf("the Save form has no name field: %s", html)
	}
	if strings.Contains(html, `data-save-author`) {
		t.Errorf("the Save form still asks for an author: %s", html)
	}
}
