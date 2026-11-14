package savedstates

import (
	"strings"
	"testing"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// at is a moment in the browser's own zone, so the row's time is the one the
// test states whatever the machine's zone is.
func at(day, hour int) time.Time {
	return time.Date(2026, 11, day, hour, 4, 0, 0, time.Local)
}

// TestScreen_RowsAndActions: a row carries the time, the name, the author and
// the two actions; a published one names its Release and its note (task 84).
func TestScreen_RowsAndActions(t *testing.T) {
	s := New()
	s.Loaded = true
	s.Rows = []Row{
		{Number: 2, Title: "Second save", Author: "Ada", SavedAt: at(13, 21), Released: true, Release: 3, Note: "for the demo"},
		{Number: 1, Title: "First save", Author: "Bob", SavedAt: at(12, 10)},
	}
	html := app.HTMLString(s.Render())

	for _, want := range []string{
		`data-history-screen`,
		`data-history-list`,
		`data-saved-state-row="2"`,
		`data-saved-state-time="2"`,
		"13.11.2026 21:04",
		"Second save",
		"Ada",
		`data-saved-state-release="2"`,
		"Release 3 · for the demo",
		"Released",
		`data-saved-state-row="1"`,
		"First save",
		"Bob",
		">Load<",
		">Make Release<",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the screen does not hold %q: %s", want, html)
		}
	}
	if strings.Contains(html, `data-saved-state-release="1"`) {
		t.Errorf("a saved state no Release published names one: %s", html)
	}
}

// TestScreen_LoadingAndEmpty: the list says what it waits for, and that a Draft
// has nothing saved, instead of showing no row at all (task 84).
func TestScreen_LoadingAndEmpty(t *testing.T) {
	s := New()
	if html := app.HTMLString(s.Render()); !strings.Contains(html, "Loading saved states…") {
		t.Errorf("the screen before its first answer: %s", html)
	}
	s.Loaded = true
	if html := app.HTMLString(s.Render()); !strings.Contains(html, "Nothing has been saved yet.") {
		t.Errorf("the screen of a Draft never saved: %s", html)
	}
}

// TestScreen_RenameInPlace: the name is edited in place, with a tick that saves
// it and a cross that drops it, and a load in flight disables every Load
// (task 84).
func TestScreen_RenameInPlace(t *testing.T) {
	s := New()
	s.Loaded = true
	s.Rows = []Row{{Number: 1, Title: "First save", Author: "Ada", SavedAt: at(12, 10)}}
	s.renaming, s.renameText = 1, "Renamed"
	html := app.HTMLString(s.Render())
	for _, want := range []string{
		`data-saved-state-rename="1"`,
		`aria-label="Save the name"`,
		`aria-label="Drop the new name"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the editor of the name does not hold %q: %s", want, html)
		}
	}

	s.renaming = 0
	s.Loading = true
	html = app.HTMLString(s.Render())
	if !strings.Contains(html, `disabled`) || !strings.Contains(html, "Replace the Draft with this saved state") {
		t.Errorf("a load in flight leaves Load enabled: %s", html)
	}
}

// TestScreen_BackIsTheWayToTheDraft: the screen names its own title and offers
// the way back, which the mode wires (task 84).
func TestScreen_BackIsTheWayToTheDraft(t *testing.T) {
	s := New()
	s.Loaded = true
	html := app.HTMLString(s.Render())
	if !strings.Contains(html, "Saved states") || !strings.Contains(html, `aria-label="Back to the Draft"`) {
		t.Errorf("the screen's own header: %s", html)
	}
}
