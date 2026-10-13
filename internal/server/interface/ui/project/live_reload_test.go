package project

import (
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestMergeEditFields_KeepsEditedFieldsAndFollowsServerForOthers(t *testing.T) {
	old := editFieldsOf(apiv0.WidgetResponse{Name: "pump", Position: apiv0.PositionResponse{X: 10, Y: 20}})
	current := editFieldsOf(apiv0.WidgetResponse{Name: "pump-renamed", Position: apiv0.PositionResponse{X: 30, Y: 40}})
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
