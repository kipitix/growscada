package scene

import "testing"

func TestNewSceneName_Valid(t *testing.T) {
	name, err := NewSceneName("main_overview")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name.String() != "main_overview" {
		t.Errorf("expected %q, got %q", "main_overview", name.String())
	}
}

func TestNewSceneName_Empty(t *testing.T) {
	_, err := NewSceneName("")
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

func TestSceneName_String(t *testing.T) {
	cases := []string{"boiler_room", "OVERVIEW", "scene_01"}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			name, err := NewSceneName(input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name.String() != input {
				t.Errorf("expected %q, got %q", input, name.String())
			}
		})
	}
}
