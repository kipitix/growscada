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
		if et == EventResync {
			t.Errorf("EventResync %q is a contract event type", EventResync)
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
