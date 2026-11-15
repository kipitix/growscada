package eventlog

import (
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

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

// TestEventTypeInfo_CoversTheContract keeps the log's table in step with the
// contract's event types, in both directions.
func TestEventTypeInfo_CoversTheContract(t *testing.T) {
	known := make(map[string]bool)
	for _, et := range apiv0.EventTypes() {
		known[et] = true
		if _, ok := eventTypeInfoByType[et]; !ok {
			t.Errorf("contract event type %q has no entry in eventTypeInfoByType", et)
		}
	}
	for et := range eventTypeInfoByType {
		if !known[et] {
			t.Errorf("eventTypeInfoByType has %q, which the contract does not define", et)
		}
	}
}

// TestEventResync_IsNoContractEventType: the synthetic type of the UI must
// never be confused with one the server sends.
func TestEventResync_IsNoContractEventType(t *testing.T) {
	for _, et := range apiv0.EventTypes() {
		if et == EventResync || et == EventProjectResync {
			t.Errorf("the resync %q is a contract event type", et)
		}
	}
}

func TestEventTypeInfo_Describe(t *testing.T) {
	if got := eventTypeInfoByType[apiv0.EventSceneCreated].describe("x"); got != "Scene x created" {
		t.Errorf("got %q", got)
	}
	if got := eventTypeInfoByType[apiv0.EventSystemReady].describe(""); got != "Server ready" {
		t.Errorf("an event without an id: got %q", got)
	}
}

// TestEventRef: a saved state event is described by its number, not by the ID of
// the Project or Library it names (task 72_2; ADR 0021).
func TestEventRef(t *testing.T) {
	savedState := apiv0.Event{Type: apiv0.EventSavedStateCreated, ID: "project-id", SavedState: 3}
	if got := eventTypeInfoByType[savedState.Type].describe(eventRef(savedState)); got != "Saved state 3 saved" {
		t.Errorf("a saved state saved: got %q", got)
	}
	renamed := apiv0.Event{Type: apiv0.EventSavedStateTitleUpdated, ID: "library-id", SavedState: 4}
	if got := eventTypeInfoByType[renamed.Type].describe(eventRef(renamed)); got != "Saved state 4 retitled" {
		t.Errorf("a saved state retitled: got %q", got)
	}
	scene := apiv0.Event{Type: apiv0.EventSceneCreated, ID: "scene-id"}
	if got := eventRef(scene); got != "scene-id" {
		t.Errorf("an event without a saved state: got %q", got)
	}
}
