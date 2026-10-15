package eventlog

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
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
		{apiv0.Event{Type: EventResync}, AllTargets},
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
