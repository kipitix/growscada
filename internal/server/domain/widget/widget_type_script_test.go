package widget

import "testing"

func TestNewScript_Valid(t *testing.T) {
	const code = "function render(v) { return v * 2; }"
	s, err := NewScript(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.String() != code {
		t.Errorf("expected %q, got %q", code, s.String())
	}
}

func TestNewScript_Empty(t *testing.T) {
	s, err := NewScript("")
	if err != nil {
		t.Fatalf("unexpected error for empty script: %v", err)
	}
	if s.String() != "" {
		t.Errorf("expected empty string, got %q", s.String())
	}
}

func TestScript_String(t *testing.T) {
	const code = "-- lua script\nreturn value"
	s, _ := NewScript(code)
	if s.String() != code {
		t.Errorf("String() mismatch: expected %q, got %q", code, s.String())
	}
}
