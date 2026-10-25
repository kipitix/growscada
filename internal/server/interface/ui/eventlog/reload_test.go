package eventlog

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
)

func TestReloadTargetsFor(t *testing.T) {
	sceneID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	tests := []struct {
		ev   apiv0.Event
		want ReloadTargets
	}{
		{apiv0.Event{Type: apiv0.EventWidgetTypeCreated}, ReloadTargets{WidgetTypes: true}},
		{apiv0.Event{Type: apiv0.EventWidgetTypeUpdated}, ReloadTargets{WidgetTypes: true}},
		{apiv0.Event{Type: apiv0.EventWidgetTypeDeleted}, ReloadTargets{WidgetTypes: true}},
		{apiv0.Event{Type: apiv0.EventSceneCreated}, ReloadTargets{Scenes: true}},
		{apiv0.Event{Type: apiv0.EventSceneUpdated}, ReloadTargets{Scenes: true}},
		{apiv0.Event{Type: apiv0.EventSceneDeleted}, ReloadTargets{Scenes: true}},
		{apiv0.Event{Type: apiv0.EventWidgetCreated, SceneID: sceneID.String()}, ReloadTargets{Scene: sceneID}},
		{apiv0.Event{Type: apiv0.EventWidgetUpdated, SceneID: sceneID.String()}, ReloadTargets{Scene: sceneID}},
		{apiv0.Event{Type: apiv0.EventWidgetDeleted, SceneID: sceneID.String()}, ReloadTargets{Scene: sceneID}},
		// Without its Scene (an older server) or with one that does not parse.
		{apiv0.Event{Type: apiv0.EventWidgetUpdated}, ReloadTargets{Scenes: true, Widgets: true}},
		{apiv0.Event{Type: apiv0.EventWidgetDeleted, SceneID: "not-a-uuid"}, ReloadTargets{Scenes: true, Widgets: true}},
		{apiv0.Event{Type: apiv0.EventTagCreated}, ReloadTargets{Tags: true}},
		{apiv0.Event{Type: apiv0.EventTagUpdated, Tag: &apiv0.TagResponse{ID: sceneID}}, ReloadTargets{}},
		{apiv0.Event{Type: apiv0.EventTagUpdated}, ReloadTargets{Tags: true}},
		{apiv0.Event{Type: apiv0.EventTagDeleted}, ReloadTargets{Tags: true}},
		{apiv0.Event{Type: EventResync}, ReloadTargets{Projects: true, WidgetTypes: true}},
		{apiv0.Event{Type: EventProjectResync}, ReloadTargets{Scenes: true, Widgets: true, Tags: true}},
		{apiv0.Event{Type: apiv0.EventProjectCreated}, ReloadTargets{Projects: true}},
		{apiv0.Event{Type: apiv0.EventProjectUpdated}, ReloadTargets{Projects: true}},
		{apiv0.Event{Type: apiv0.EventProjectDeleted}, ReloadTargets{Projects: true}},
		{apiv0.Event{Type: apiv0.EventClientConnected}, ReloadTargets{}},
		{apiv0.Event{Type: apiv0.EventSystemReady}, ReloadTargets{}},
		{apiv0.Event{Type: "unknown"}, ReloadTargets{}},
		{apiv0.Event{}, ReloadTargets{}},
	}
	for _, tt := range tests {
		if got := ReloadTargetsFor(tt.ev); got != tt.want {
			t.Errorf("ReloadTargetsFor(%+v) = %+v, want %+v", tt.ev, got, tt.want)
		}
	}
}

// TestReloadTargetsFor_CoversEveryKnownEvent guards the table against a new
// aggregate event type added to the event log but not routed.
func TestReloadTargetsFor_CoversEveryKnownEvent(t *testing.T) {
	for eventType, info := range eventTypeInfoByType {
		if info.category == categoryClient || eventType == apiv0.EventSystemReady {
			continue // change no server-backed list
		}
		ev := apiv0.Event{Type: eventType}
		if eventType == apiv0.EventTagUpdated {
			// With the Tag's state the modes apply it instead of reloading;
			// without it, the Tags are reread.
			if got := ReloadTargetsFor(apiv0.Event{Type: eventType, Tag: &apiv0.TagResponse{}}); got != (ReloadTargets{}) {
				t.Errorf("%q with its Tag reloads %+v", eventType, got)
			}
		}
		if ReloadTargetsFor(ev) == (ReloadTargets{}) {
			t.Errorf("%q reloads nothing", eventType)
		}
	}
}

func TestReloadTargets_WidgetsOf(t *testing.T) {
	selected, other := uuid.New(), uuid.New()

	for name, tc := range map[string]struct {
		targets  ReloadTargets
		selected uuid.UUID
		want     bool
	}{
		"every Scene's":      {ReloadTargets{Widgets: true}, selected, true},
		"the selected one's": {ReloadTargets{Scene: selected}, selected, true},
		"another Scene's":    {ReloadTargets{Scene: other}, selected, false},
		"none selected":      {ReloadTargets{}, uuid.Nil, false},
		"no Widgets":         {ReloadTargets{Scenes: true}, selected, false},
	} {
		if got := tc.targets.WidgetsOf(tc.selected); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

func TestApplyToTags(t *testing.T) {
	id := uuid.New()
	known := tagstate.Tags{}.Merge(0, []apiv0.TagResponse{{ID: id, Value: "1", Version: 5}})
	value := func(ts tagstate.Tags) string { tag, _ := ts.Find(id); return tag.Value }

	updated := ApplyToTags(known, apiv0.Event{Type: apiv0.EventTagUpdated, ID: id.String(), Tag: &apiv0.TagResponse{ID: id, Value: "2", Version: 6}})
	if value(updated) != "2" {
		t.Errorf("tag_updated: got %q, want its state", value(updated))
	}
	if got := ApplyToTags(known, apiv0.Event{Type: apiv0.EventTagUpdated, ID: id.String()}); value(got) != "1" {
		t.Errorf("tag_updated without a state: got %q, want the Tags as they were", value(got))
	}
	forgot := ApplyToTags(updated, apiv0.Event{Type: EventProjectResync})
	if got := ApplyToTags(forgot, apiv0.Event{Type: apiv0.EventTagUpdated, ID: id.String(), Tag: &apiv0.TagResponse{ID: id, Value: "fresh", Version: 1}}); value(got) != "fresh" {
		t.Errorf("after a resync of the project's stream: got %q, want a state of the started-over Versions", value(got))
	}
	// The server's stream carries no Tags: its resync leaves them as they are.
	if got := ApplyToTags(updated, apiv0.Event{Type: EventResync}); value(ApplyToTags(got, apiv0.Event{Type: apiv0.EventTagUpdated, ID: id.String(), Tag: &apiv0.TagResponse{ID: id, Value: "old", Version: 1}})) != "2" {
		t.Errorf("after a resync of the server's stream the Versions must be kept")
	}
	if got := ApplyToTags(known, apiv0.Event{Type: apiv0.EventSceneCreated}); value(got) != "1" {
		t.Errorf("another event: got %q", value(got))
	}
}
