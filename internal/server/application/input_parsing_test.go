package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// Create and Update of a resource parse the client's fields with one function,
// so each invalid field must give the same answer through both: invalid input,
// with the field named in the message.

// assertInvalidField checks that err is invalid input and names field.
func assertInvalidField(t *testing.T, err error, field string) {
	t.Helper()
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got: %v", err)
	}
	if !strings.Contains(err.Error(), field+":") {
		t.Errorf("expected the message to name %q, got: %v", field, err)
	}
}

func TestSceneInput_InvalidField_IsInvalidInputOnCreateAndUpdate(t *testing.T) {
	cases := map[string]struct {
		field  string
		modify func(*appdto.SceneInput)
	}{
		"empty name":  {"name", func(in *appdto.SceneInput) { in.Name = "" }},
		"zero width":  {"width, height", func(in *appdto.SceneInput) { in.Width = 0 }},
		"zero height": {"width, height", func(in *appdto.SceneInput) { in.Height = 0 }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cleanScenes(t)
			svc := newSceneService()
			ctx := context.Background()
			sc := mustCreateScene(t, svc)
			input := testCreateSceneInput
			tc.modify(&input)

			_, err := svc.CreateScene(ctx, input)
			assertInvalidField(t, err, tc.field)

			_, err = svc.UpdateScene(ctx, sc.ID, sc.Version, input)
			assertInvalidField(t, err, tc.field)
		})
	}
}

func TestWidgetInput_InvalidField_IsInvalidInputOnCreateAndUpdate(t *testing.T) {
	cases := map[string]struct {
		field  string
		modify func(*appdto.WidgetInput)
	}{
		"empty name":      {"name", func(in *appdto.WidgetInput) { in.Name = "" }},
		"zero width":      {"size", func(in *appdto.WidgetInput) { in.Width = 0 }},
		"origin outside":  {"origin", func(in *appdto.WidgetInput) { in.OriginX = 2 }},
		"missing type_id": {"type_id", func(in *appdto.WidgetInput) { in.TypeID = uuid.Nil }},
		"bad port name": {"port_bindings", func(in *appdto.WidgetInput) {
			in.PortBindings = []appdto.PortBinding{{PortName: "", TagID: uuid.New()}}
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cleanScenes(t)
			svc := newSceneService()
			ctx := context.Background()
			sc := mustCreateScene(t, svc)
			created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, testCreateWidgetInput)
			if err != nil {
				t.Fatalf("CreateWidget: %v", err)
			}
			input := testCreateWidgetInput
			tc.modify(&input)

			_, err = svc.CreateWidget(ctx, sc.ID, created.SceneVersion, input)
			assertInvalidField(t, err, tc.field)

			_, err = svc.UpdateWidget(ctx, sc.ID, created.ID, created.SceneVersion, input)
			assertInvalidField(t, err, tc.field)
		})
	}
}

func TestWidgetInput_NegativeSceneVersion_IsInvalidInputOnCreateAndUpdate(t *testing.T) {
	cleanScenes(t)
	svc := newSceneService()
	ctx := context.Background()
	sc := mustCreateScene(t, svc)
	created, err := svc.CreateWidget(ctx, sc.ID, sc.Version, testCreateWidgetInput)
	if err != nil {
		t.Fatalf("CreateWidget: %v", err)
	}

	_, err = svc.CreateWidget(ctx, sc.ID, -1, testCreateWidgetInput)
	assertInvalidField(t, err, "scene_version")

	_, err = svc.UpdateWidget(ctx, sc.ID, created.ID, -1, testCreateWidgetInput)
	assertInvalidField(t, err, "scene_version")
}

func TestWidgetTypeInput_InvalidField_IsInvalidInputOnCreateAndUpdate(t *testing.T) {
	cases := map[string]struct {
		field  string
		modify func(*appdto.WidgetTypeInput)
	}{
		"empty name":         {"name", func(in *appdto.WidgetTypeInput) { in.Name = "" }},
		"unknown language":   {"script_language", func(in *appdto.WidgetTypeInput) { in.ScriptLanguage = "ruby" }},
		"zero default width": {"default_width, default_height", func(in *appdto.WidgetTypeInput) { in.DefaultWidth = 0 }},
		"bad port name": {"input_ports", func(in *appdto.WidgetTypeInput) {
			in.InputPorts = []appdto.InputPort{{Name: "not an identifier"}}
		}},
		"unknown type hint": {"input_ports", func(in *appdto.WidgetTypeInput) {
			in.InputPorts = []appdto.InputPort{{Name: "value", TypeHint: "unknown"}}
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cleanWidgetTypes(t)
			svc := newWidgetTypeService()
			ctx := context.Background()
			created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
			if err != nil {
				t.Fatalf("CreateWidgetType: %v", err)
			}
			input := testCreateWidgetTypeInput
			tc.modify(&input)

			_, err = svc.CreateWidgetType(ctx, input)
			assertInvalidField(t, err, tc.field)

			_, err = svc.UpdateWidgetType(ctx, created.ID, created.Version, input)
			assertInvalidField(t, err, tc.field)
		})
	}
}

func TestWidgetTypeInput_DuplicatePortNames_IsInvalidInputOnCreateAndUpdate(t *testing.T) {
	cleanWidgetTypes(t)
	svc := newWidgetTypeService()
	ctx := context.Background()
	created, err := svc.CreateWidgetType(ctx, testCreateWidgetTypeInput)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	input := testCreateWidgetTypeInput
	input.InputPorts = []appdto.InputPort{{Name: "value"}, {Name: "value"}}

	_, err = svc.CreateWidgetType(ctx, input)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("CreateWidgetType: expected ErrInvalidInput, got: %v", err)
	}

	_, err = svc.UpdateWidgetType(ctx, created.ID, created.Version, input)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Errorf("UpdateWidgetType: expected ErrInvalidInput, got: %v", err)
	}
}
