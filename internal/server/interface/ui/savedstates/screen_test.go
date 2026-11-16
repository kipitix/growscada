package savedstates

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// at is a moment in the browser's own zone, so the row's time is the one the
// test states whatever the machine's zone is.
func at(day, hour int) time.Time {
	return time.Date(2026, 11, day, hour, 4, 0, 0, time.Local)
}

// TestScreen_RowsAndActions: a row carries the time, the name, the author and
// the Load action (task 84, task 85_7).
func TestScreen_RowsAndActions(t *testing.T) {
	s := New()
	s.Loaded = true
	s.Rows = []Row{
		{Number: 2, Title: "Second save", Author: "Ada", SavedAt: at(13, 21)},
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
		`data-saved-state-row="1"`,
		"First save",
		"Bob",
		">Load<",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the screen does not hold %q: %s", want, html)
		}
	}
	// A saved state is loaded, never published: the Release is gone from the
	// model (task 85_7).
	for _, unwant := range []string{`data-saved-state-release`, "Make Release", ">Released<"} {
		if strings.Contains(html, unwant) {
			t.Errorf("the screen still shows %q: %s", unwant, html)
		}
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

// TestScreen_LabelsChipsAndMenu: a saved state shows a colored chip per label
// and a "+ label" control that opens the menu of every label, the ones it
// carries marked (task 85_3; ADR 0025).
func TestScreen_LabelsChipsAndMenu(t *testing.T) {
	marked := apiv0.LabelResponse{ID: uuid.New(), Name: "ready_for_deploy", Color: "#22c55e"}
	other := apiv0.LabelResponse{ID: uuid.New(), Name: "ready_for_import"}
	s := New()
	s.Loaded = true
	s.AllLabels = []apiv0.LabelResponse{marked, other}
	s.Rows = []Row{{Number: 1, Title: "First save", Author: "Ada", SavedAt: at(12, 10),
		Labels: []apiv0.LabelResponse{marked}}}

	html := app.HTMLString(s.Render())
	for _, want := range []string{
		`data-label-chip="#22c55e"`,
		`data-label-id="` + marked.ID.String() + `"`,
		"ready_for_deploy",
		`data-label-add="1"`,
		">+ label<",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the labels of the row do not hold %q: %s", want, html)
		}
	}
	if strings.Contains(html, "data-label-menu") {
		t.Errorf("the menu is open before the control is clicked: %s", html)
	}

	s.labelMenu = 1
	html = app.HTMLString(s.Render())
	for _, want := range []string{
		`data-label-menu="1"`,
		`data-label-option="` + marked.ID.String() + `"`,
		`data-label-checked`,
		`data-label-option="` + other.ID.String() + `"`,
		"ready_for_import",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the menu does not hold %q: %s", want, html)
		}
	}
}

// TestScreen_LabelMenuWithNoLabels: an Organization without labels says so in
// the menu and offers the Labels page (task 85_3).
func TestScreen_LabelMenuWithNoLabels(t *testing.T) {
	s := New()
	s.Loaded = true
	s.Rows = []Row{{Number: 1, Title: "First save", SavedAt: at(12, 10)}}
	s.labelMenu = 1
	html := app.HTMLString(s.Render())
	for _, want := range []string{`data-label-menu="1"`, "No labels yet", "Manage labels"} {
		if !strings.Contains(html, want) {
			t.Errorf("the empty menu does not hold %q: %s", want, html)
		}
	}
}

// TestScreen_Reset: closing the screen drops the name being edited and the
// open labels menu (task 84, task 85_3).
func TestScreen_Reset(t *testing.T) {
	s := New()
	s.renaming, s.renameText, s.labelMenu = 2, "New name", 3
	s.Reset()
	if s.renaming != 0 || s.renameText != "" || s.labelMenu != 0 {
		t.Errorf("the screen kept its own state: %d %q %d", s.renaming, s.renameText, s.labelMenu)
	}
}
