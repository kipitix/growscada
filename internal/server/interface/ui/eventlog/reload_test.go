package eventlog

import "testing"

func TestReloadTargetsFor(t *testing.T) {
	tests := map[string]ReloadTargets{
		"widget_type_created": {WidgetTypes: true},
		"widget_type_updated": {WidgetTypes: true},
		"widget_type_deleted": {WidgetTypes: true},
		"scene_created":       {Scenes: true},
		"scene_updated":       {Scenes: true},
		"scene_deleted":       {Scenes: true},
		"widget_created":      {Scenes: true, Widgets: true},
		"widget_updated":      {Scenes: true, Widgets: true},
		"widget_deleted":      {Scenes: true, Widgets: true},
		"tag_created":         {Tags: true},
		"tag_updated":         {Tags: true},
		"tag_deleted":         {Tags: true},
		EventResync:           AllTargets,
		"client_connected":    {},
		"system_ready":        {},
		"":                    {},
	}
	for eventType, want := range tests {
		if got := ReloadTargetsFor(eventType); got != want {
			t.Errorf("ReloadTargetsFor(%q) = %+v, want %+v", eventType, got, want)
		}
	}
}

// TestReloadTargetsFor_CoversEveryKnownEvent guards the table against a new
// aggregate event type added to the event log but not routed.
func TestReloadTargetsFor_CoversEveryKnownEvent(t *testing.T) {
	for eventType, info := range eventTypeInfoByType {
		if info.category == categoryClient {
			continue // connections change no server-backed list
		}
		if ReloadTargetsFor(eventType) == (ReloadTargets{}) {
			t.Errorf("%q reloads nothing", eventType)
		}
	}
}
