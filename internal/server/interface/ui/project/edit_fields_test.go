package project

import (
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestMergeWidgetEditFields_KeepsEditedFieldsAndFollowsServerForOthers(t *testing.T) {
	old := widgetEditFieldsOf(apiv0.WidgetResponse{Name: "pump", Position: apiv0.PositionResponse{X: 10, Y: 20}})
	current := widgetEditFieldsOf(apiv0.WidgetResponse{Name: "pump-renamed", Position: apiv0.PositionResponse{X: 30, Y: 40}})
	local := old
	local.posX = "99" // unsaved edit

	got := mergeWidgetEditFields(local, old, current)

	if got.posX != "99" {
		t.Errorf("edited posX: got %q, want the unsaved %q", got.posX, "99")
	}
	if got.name != "pump-renamed" || got.posY != "40.0" {
		t.Errorf("unedited fields must follow the server, got name=%q posY=%q", got.name, got.posY)
	}
}

func TestMergeSceneEditFields_KeepsEditedFieldsAndFollowsServerForOthers(t *testing.T) {
	old := sceneEditFieldsOf(apiv0.SceneResponse{Name: "Main", Width: 800, Height: 600, BackgroundHTML: "<p>a</p>"})
	current := sceneEditFieldsOf(apiv0.SceneResponse{Name: "Main renamed", Width: 1024, Height: 768, BackgroundHTML: "<p>b</p>"})
	local := old
	local.width = "1920" // unsaved edit

	got := mergeSceneEditFields(local, old, current)

	want := sceneEditFields{name: "Main renamed", width: "1920", height: "768", background: "<p>b</p>"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
