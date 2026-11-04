package application_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
)

// buildProjectFile builds the ProjectFile of mainProject, as a Deploy does.
func buildProjectFile(t *testing.T) string {
	t.Helper()
	file, err := application.NewProjectFileBuilder(contentOver(newScopes())).Build(editlock.System(context.Background()), mainProject)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	encoded, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal the ProjectFile: %v", err)
	}
	return string(encoded)
}

func TestBuild_IsCanonical(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	cleanPins(t)
	ctx := editlock.System(context.Background())

	mustCreateTag(t, "b-tag", "integer")
	mustCreateTag(t, "a-tag", "boolean")
	created, err := newDeviceService().CreateDevice(ctx, mainProject, appdto.DeviceInput{
		Name: "pump", Protocol: "simulation", Config: []byte(`{"b": 1, "a": {"y": 2, "x": 3}}`),
	})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}

	first := buildProjectFile(t)
	second := buildProjectFile(t)
	if first != second {
		t.Errorf("the same Draft built twice: %s, then %s", first, second)
	}

	var file struct {
		Tags    []json.RawMessage `json:"tags"`
		Devices []struct {
			ID     string          `json:"id"`
			Config json.RawMessage `json:"config"`
		} `json:"devices"`
	}
	if err := json.Unmarshal([]byte(first), &file); err != nil {
		t.Fatalf("decode the ProjectFile: %v", err)
	}
	if len(file.Tags) != 2 {
		t.Errorf("the tags of the ProjectFile: %d, want both", len(file.Tags))
	}
	if got := string(file.Devices[0].Config); got != `{"a":{"x":3,"y":2},"b":1}` {
		t.Errorf("the config of the device: %s, want its keys sorted", got)
	}
	if file.Devices[0].ID != created.ID.String() {
		t.Errorf("the device of the ProjectFile: %s, want %s", file.Devices[0].ID, created.ID)
	}
}

func TestBuild_CarriesOnlyTheWidgetTypesItsWidgetsUse(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	cleanPins(t)
	ctx := editlock.System(context.Background())

	used, _ := pinNewWidgetType(t, mainProject, appdto.WidgetTypeInput{Name: "used"})
	pinNewWidgetType(t, mainProject, appdto.WidgetTypeInput{Name: "unused"})

	scenes := newSceneService()
	scene := mustCreateScene(t, scenes)
	input := testCreateWidgetInput
	input.TypeID = used.ID
	if _, err := scenes.CreateWidget(ctx, mainProject, scene.ID, scene.Version, input); err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	file, err := application.NewProjectFileBuilder(contentOver(newScopes())).Build(ctx, mainProject)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(file.WidgetTypes) != 1 || file.WidgetTypes[0].ID != used.ID {
		t.Errorf("the widget types of the ProjectFile: %+v, want only %s", file.WidgetTypes, used.ID)
	}
	if len(file.LibraryPins) != 2 {
		t.Errorf("the pins of the ProjectFile: %+v, want both, as provenance", file.LibraryPins)
	}
}

func TestBuild_AWidgetBoundToATagTheDraftLacks_IsAnError(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	cleanPins(t)
	ctx := editlock.System(context.Background())

	wt, _ := pinNewWidgetType(t, mainProject, appdto.WidgetTypeInput{
		Name: "gauge", InputPorts: []appdto.InputPort{{Name: "value", TypeHint: "integer"}},
	})
	speed := mustCreateTag(t, "speed", "integer")
	scenes := newSceneService()
	scene := mustCreateScene(t, scenes)
	input := testCreateWidgetInput
	input.TypeID = wt.ID
	input.PortBindings = []appdto.PortBinding{{PortName: "value", TagID: speed.ID}}
	if _, err := scenes.CreateWidget(ctx, mainProject, scene.ID, scene.Version, input); err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	// The Tag goes behind the service's back: the Widget keeps its binding.
	if _, err := testDB.ExecContext(ctx, "DELETE FROM engineering.tags WHERE id = $1", speed.ID); err != nil {
		t.Fatalf("delete tag: %v", err)
	}
	// The Scene goes too, so the next test starts from a Draft that builds.
	t.Cleanup(func() {
		if _, err := testDB.ExecContext(editlock.System(context.Background()),
			"DELETE FROM engineering.scenes WHERE id = $1", scene.ID); err != nil {
			t.Fatalf("clean the scene: %v", err)
		}
	})

	if _, err := application.NewProjectFileBuilder(contentOver(newScopes())).Build(ctx, mainProject); err == nil {
		t.Error("a widget bound to a tag the draft lacks: the build passed, want an error")
	}
}

func TestBuild_AWidgetOfANoLongerPinnedType_IsAnError(t *testing.T) {
	cleanDevices(t)
	cleanScenes(t)
	cleanPins(t)
	ctx := editlock.System(context.Background())
	// The Draft keeps a Widget of a type no pinned release holds, so the Scene
	// and the pins go with the test: the next one starts from a buildable Draft.
	t.Cleanup(func() {
		cleanScenes(t)
		cleanPins(t)
	})

	// The pin goes behind the service's back: the Draft keeps a Widget of a
	// type no pinned release holds.
	wt, lib := pinNewWidgetType(t, mainProject, appdto.WidgetTypeInput{Name: "orphan-maker"})
	scenes := newSceneService()
	scene := mustCreateScene(t, scenes)
	input := testCreateWidgetInput
	input.TypeID = wt.ID
	if _, err := scenes.CreateWidget(ctx, mainProject, scene.ID, scene.Version, input); err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}
	if _, err := testDB.ExecContext(ctx,
		"DELETE FROM engineering.library_pins WHERE project_id = $1 AND library_id = $2", mainProject, lib.ID); err != nil {
		t.Fatalf("depin: %v", err)
	}

	if _, err := application.NewProjectFileBuilder(contentOver(newScopes())).Build(ctx, mainProject); err == nil {
		t.Error("a widget of a type no pinned release holds: the build passed, want an error")
	}
}
