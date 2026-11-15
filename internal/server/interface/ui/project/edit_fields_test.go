package project

import (
	"context"
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

// Saving one field of the properties panel does not round the others: the
// panel shows the geometry rounded, but a field the user never touched is
// saved with the Widget's own value (task 61).
func TestSaveWidgetGeometry_KeepsTheFieldsTheUserDidNotTouchExact(t *testing.T) {
	f, sceneID, ids, _ := moveFixture(t, 1)
	w := f.widget(sceneID, ids[0])
	w.Rotation = apiv0.RotationResponse{Degrees: 37.2846}
	w.Origin = apiv0.OriginResponse{X: 0.3333, Y: 0.6666}
	if _, err := f.api.UpdateWidget(context.Background(), sceneID, ids[0], widgetRequest(w, f.scene(sceneID).Version)); err != nil {
		t.Fatalf("update widget: %v", err)
	}
	f.p.loadWidgets(f.p.compoCtx)
	f.run()
	f.p.selectOnly(f.p.compoCtx, ids[0])

	if got := f.p.widgetEdit.rotation; got != "37.3" {
		t.Fatalf("the panel shows rotation %q, want the rounded 37.3", got)
	}
	f.p.widgetEdit.posX = "150.0" // only the position is edited
	f.p.saveWidgetGeometry(f.p.compoCtx)
	f.run()

	f.noToasts()
	saved := f.widget(sceneID, ids[0])
	if saved.Position.X != 150 {
		t.Errorf("position X %v, want 150", saved.Position.X)
	}
	if saved.Rotation.Degrees != 37.2846 {
		t.Errorf("rotation %v: the save rounded a field the user did not touch", saved.Rotation.Degrees)
	}
	if saved.Origin.X != 0.3333 || saved.Origin.Y != 0.6666 {
		t.Errorf("origin %+v: the save rounded a field the user did not touch", saved.Origin)
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
