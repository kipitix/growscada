package scene

import (
	"strings"
	"testing"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/version"
)

// makeTestScene is a helper that builds a Scene with sensible defaults.
func makeTestScene(t *testing.T) Scene {
	t.Helper()
	sceneID := id.NewID[Scene]()
	name, _ := NewSceneName("test_scene")
	size, _ := NewSceneSize(1024, 768)
	bg := NewBackgroundHTML("<div></div>")
	return NewScene(sceneID, name, size, bg, version.Initial[Scene]())
}

func TestNewScene_FieldsAreSet(t *testing.T) {
	sceneID := id.NewID[Scene]()
	name, _ := NewSceneName("boiler_room")
	size, _ := NewSceneSize(1920, 1080)
	bg := NewBackgroundHTML("<svg></svg>")
	ver, _ := version.New[Scene](version.WithNumber[Scene](2))

	s := NewScene(sceneID, name, size, bg, ver)

	if s.ID() != sceneID {
		t.Errorf("ID mismatch: expected %v, got %v", sceneID, s.ID())
	}
	if s.Name() != name {
		t.Errorf("Name mismatch: expected %v, got %v", name, s.Name())
	}
	if s.Size() != size {
		t.Errorf("Size mismatch: expected %v, got %v", size, s.Size())
	}
	if s.BackgroundHTML() != bg {
		t.Errorf("BackgroundHTML mismatch: expected %v, got %v", bg, s.BackgroundHTML())
	}
	if s.Version() != ver {
		t.Errorf("Version mismatch: expected %v, got %v", ver, s.Version())
	}
}

func TestNewScene_InitialVersion(t *testing.T) {
	s := makeTestScene(t)
	if s.Version() != version.Initial[Scene]() {
		t.Errorf("expected initial version %v, got %v", version.Initial[Scene](), s.Version())
	}
}

func TestNewScene_String_NonEmpty(t *testing.T) {
	s := makeTestScene(t)
	str := s.String()
	if str == "" {
		t.Error("expected non-empty String() output")
	}
}

func TestNewScene_String_ContainsNameAndSize(t *testing.T) {
	name, _ := NewSceneName("overview")
	size, _ := NewSceneSize(800, 600)
	s := NewScene(id.NewID[Scene](), name, size, NewBackgroundHTML(""), version.Initial[Scene]())

	str := s.String()
	if !strings.Contains(str, "overview") {
		t.Errorf("String() should contain scene name %q, got %q", "overview", str)
	}
	if !strings.Contains(str, "800") {
		t.Errorf("String() should contain width 800, got %q", str)
	}
	if !strings.Contains(str, "600") {
		t.Errorf("String() should contain height 600, got %q", str)
	}
}

func TestNewScene_EmptyBackgroundHTML(t *testing.T) {
	s := NewScene(id.NewID[Scene](), mustSceneName(t, "empty_bg"), mustSceneSize(t, 100, 100), NewBackgroundHTML(""), version.Initial[Scene]())
	if s.BackgroundHTML().Content() != "" {
		t.Errorf("expected empty background HTML, got %q", s.BackgroundHTML().Content())
	}
}

func TestNewScene_UniqueIDs(t *testing.T) {
	s1 := makeTestScene(t)
	s2 := makeTestScene(t)
	if s1.ID() == s2.ID() {
		t.Error("two scenes created independently must have different IDs")
	}
}

// helpers to reduce boilerplate inside table-driven sub-tests.

func mustSceneName(t *testing.T, n string) SceneName {
	t.Helper()
	name, err := NewSceneName(n)
	if err != nil {
		t.Fatalf("mustSceneName(%q): %v", n, err)
	}
	return name
}

func mustSceneSize(t *testing.T, w, h int) SceneSize {
	t.Helper()
	size, err := NewSceneSize(w, h)
	if err != nil {
		t.Fatalf("mustSceneSize(%d, %d): %v", w, h, err)
	}
	return size
}
