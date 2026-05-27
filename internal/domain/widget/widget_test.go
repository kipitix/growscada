package widget

import (
	"strings"
	"testing"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/version"
)

// makeTestWidget is a test helper that creates a Widget with the given geometry
// and sensible defaults for the remaining fields.
func makeTestWidget(t *testing.T, pos Position, size Size, origin Origin, rot Rotation) Widget {
	t.Helper()

	wID := id.NewID[Widget]()
	name, _ := NewWidgetName("test_widget")
	typeID := id.NewID[WidgetType]()
	sceneID := id.NewID[scene.Scene]()

	return NewWidget(
		wID, name, pos, size, origin, rot,
		typeID, sceneID,
		[]string{"label_a", "label_b"},
		[]id.ID[tag.Tag]{id.NewID[tag.Tag]()},
		version.Initial[Widget](),
	)
}

func TestNewWidget_FieldsAreSet(t *testing.T) {
	wID := id.NewID[Widget]()
	name, _ := NewWidgetName("flow_meter")
	pos := NewPosition(10, 20, 1)
	size, _ := NewSize(150, 75)
	origin, _ := NewOrigin(0.5, 0.5)
	rot := NewRotation(45)
	typeID := id.NewID[WidgetType]()
	sceneID := id.NewID[scene.Scene]()
	labels := []string{"pump", "main_loop"}
	tagID := id.NewID[tag.Tag]()
	tagIDs := []id.ID[tag.Tag]{tagID}
	ver := version.Initial[Widget]()

	w := NewWidget(wID, name, pos, size, origin, rot, typeID, sceneID, labels, tagIDs, ver)

	if w.ID() != wID {
		t.Errorf("ID mismatch: expected %v, got %v", wID, w.ID())
	}
	if w.Name() != name {
		t.Errorf("Name mismatch: expected %v, got %v", name, w.Name())
	}
	if w.Position() != pos {
		t.Errorf("Position mismatch: expected %v, got %v", pos, w.Position())
	}
	if w.Size() != size {
		t.Errorf("Size mismatch: expected %v, got %v", size, w.Size())
	}
	if w.Origin() != origin {
		t.Errorf("Origin mismatch: expected %v, got %v", origin, w.Origin())
	}
	if w.Rotation() != rot {
		t.Errorf("Rotation mismatch: expected %v, got %v", rot, w.Rotation())
	}
	if w.TypeID() != typeID {
		t.Errorf("TypeID mismatch: expected %v, got %v", typeID, w.TypeID())
	}
	if w.SceneID() != sceneID {
		t.Errorf("SceneID mismatch: expected %v, got %v", sceneID, w.SceneID())
	}
	if w.Version() != ver {
		t.Errorf("Version mismatch: expected %v, got %v", ver, w.Version())
	}
}

func TestNewWidget_LabelsCopied(t *testing.T) {
	original := []string{"a", "b"}
	pos := NewPosition(0, 0, 0)
	size, _ := NewSize(100, 100)
	origin, _ := NewOrigin(0, 0)
	rot := NewRotation(0)

	w := makeTestWidget(t, pos, size, origin, rot)
	_ = original // confirm we built a widget; now test copy semantics below

	wID := id.NewID[Widget]()
	name, _ := NewWidgetName("copy_test")
	typeID := id.NewID[WidgetType]()
	sceneID := id.NewID[scene.Scene]()
	labels := []string{"x", "y"}

	widget := NewWidget(wID, name, pos, size, origin, rot, typeID, sceneID, labels, nil, version.Initial[Widget]())

	labels[0] = "mutated"
	if widget.Labels()[0] != "x" {
		t.Error("NewWidget must copy the labels slice; external mutation should not affect widget")
	}
	_ = w
}

func TestNewWidget_TagIDsCopied(t *testing.T) {
	pos := NewPosition(0, 0, 0)
	size, _ := NewSize(100, 100)
	origin, _ := NewOrigin(0, 0)
	rot := NewRotation(0)
	wID := id.NewID[Widget]()
	name, _ := NewWidgetName("tag_copy_test")
	typeID := id.NewID[WidgetType]()
	sceneID := id.NewID[scene.Scene]()

	originalTagID := id.NewID[tag.Tag]()
	tagIDs := []id.ID[tag.Tag]{originalTagID}

	widget := NewWidget(wID, name, pos, size, origin, rot, typeID, sceneID, nil, tagIDs, version.Initial[Widget]())

	tagIDs[0] = id.NewID[tag.Tag]() // mutate external slice
	if widget.TagIDs()[0] != originalTagID {
		t.Error("NewWidget must copy the tagIDs slice; external mutation should not affect widget")
	}
}

func TestNewWidget_NilLabelsAndTagIDs(t *testing.T) {
	pos := NewPosition(0, 0, 0)
	size, _ := NewSize(100, 100)
	origin, _ := NewOrigin(0, 0)
	rot := NewRotation(0)
	wID := id.NewID[Widget]()
	name, _ := NewWidgetName("nil_slices")
	typeID := id.NewID[WidgetType]()
	sceneID := id.NewID[scene.Scene]()

	w := NewWidget(wID, name, pos, size, origin, rot, typeID, sceneID, nil, nil, version.Initial[Widget]())
	if len(w.Labels()) != 0 {
		t.Errorf("expected empty labels, got %v", w.Labels())
	}
	if len(w.TagIDs()) != 0 {
		t.Errorf("expected empty tagIDs, got %v", w.TagIDs())
	}
}

func TestNewWidget_String(t *testing.T) {
	pos := NewPosition(5, 10, 0)
	size, _ := NewSize(120, 80)
	origin, _ := NewOrigin(0.5, 0.5)
	rot := NewRotation(30)

	w := makeTestWidget(t, pos, size, origin, rot)
	s := w.String()

	if s == "" {
		t.Error("expected non-empty String() output")
	}
	if !strings.Contains(s, "Widget") {
		t.Errorf("String() should mention 'Widget', got %q", s)
	}
}

func TestNewWidget_TransformationMatrix(t *testing.T) {
	pos := NewPosition(0, 0, 0)
	size, _ := NewSize(100, 100)
	origin, _ := NewOrigin(0, 0)
	rot := NewRotation(0)

	w := makeTestWidget(t, pos, size, origin, rot)
	m := w.TransformationMatrix()

	// zero rotation + top-left origin → identity translation at (0,0)
	if !approxEqual(m.A(), 1) || !approxEqual(m.D(), 1) {
		t.Errorf("expected identity-like matrix, got %s", m.CSS())
	}
}
