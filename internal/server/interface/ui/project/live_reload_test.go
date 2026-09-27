package project

import "testing"

func TestReloadTargetsFor(t *testing.T) {
	tests := map[string]reloadTargets{
		"widget_type_created": {widgetTypes: true},
		"widget_type_updated": {widgetTypes: true},
		"widget_type_deleted": {widgetTypes: true},
		"scene_created":       {scenes: true},
		"scene_updated":       {scenes: true},
		"scene_deleted":       {scenes: true},
		"widget_created":      {scenes: true, widgets: true},
		"widget_updated":      {scenes: true, widgets: true},
		"widget_deleted":      {scenes: true, widgets: true},
		"tag_created":         {tags: true},
		"tag_updated":         {tags: true},
		"tag_deleted":         {tags: true},
		"client_connected":    {},
		"system_ready":        {},
		"":                    {},
	}
	for eventType, want := range tests {
		if got := reloadTargetsFor(eventType); got != want {
			t.Errorf("reloadTargetsFor(%q) = %+v, want %+v", eventType, got, want)
		}
	}
}

func TestMergeEditFields_KeepsEditedFieldsAndFollowsServerForOthers(t *testing.T) {
	old := editFieldsOf(widgetItem{Name: "pump", Position: positionDTO{X: 10, Y: 20}})
	current := editFieldsOf(widgetItem{Name: "pump-renamed", Position: positionDTO{X: 30, Y: 40}})
	local := old
	local.posX = "99" // unsaved edit

	got := mergeEditFields(local, old, current)

	if got.posX != "99" {
		t.Errorf("edited posX: got %q, want the unsaved %q", got.posX, "99")
	}
	if got.name != "pump-renamed" || got.posY != "40.0" {
		t.Errorf("unedited fields must follow the server, got name=%q posY=%q", got.name, got.posY)
	}
}
