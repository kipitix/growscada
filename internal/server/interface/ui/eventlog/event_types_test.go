package eventlog

import "testing"

// TestCategoryListsInSync guards against the two category lists drifting
// apart: a category present in one but missing from the other means either
// a filter pill with no default state, or a default state with no pill to
// toggle it — the latter is exactly how unknown-type events silently
// vanished from the log with no way to re-enable them.
func TestCategoryListsInSync(t *testing.T) {
	defaults := defaultFilters()

	for _, c := range allCategories {
		if _, ok := defaults[c]; !ok {
			t.Errorf("category %q is in allCategories but has no entry in defaultFilters()", c)
		}
	}

	inAllCategories := make(map[category]bool, len(allCategories))
	for _, c := range allCategories {
		inAllCategories[c] = true
	}
	for c := range defaults {
		if !inAllCategories[c] {
			t.Errorf("category %q is in defaultFilters() but missing from allCategories", c)
		}
	}
}

// TestCategoryOtherVisibleByDefault ensures events of an unrecognized type
// (categoryOther) are shown, not hidden, until the user opts out — unlike
// categoryValue, "unknown" isn't inherently noisy, so it shouldn't default
// to invisible.
func TestCategoryOtherVisibleByDefault(t *testing.T) {
	if !defaultFilters()[categoryOther] {
		t.Error("categoryOther should default to visible (true)")
	}
}
