package project

import "testing"

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
