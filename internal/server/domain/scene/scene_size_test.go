package scene

import (
	"strings"
	"testing"
)

func TestNewSceneSize_Valid(t *testing.T) {
	s, err := NewSceneSize(1920, 1080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Width() != 1920 {
		t.Errorf("Width: expected 1920, got %d", s.Width())
	}
	if s.Height() != 1080 {
		t.Errorf("Height: expected 1080, got %d", s.Height())
	}
}

func TestNewSceneSize_ZeroWidth(t *testing.T) {
	_, err := NewSceneSize(0, 100)
	if err == nil {
		t.Error("expected error for zero width, got nil")
	}
}

func TestNewSceneSize_NegativeWidth(t *testing.T) {
	_, err := NewSceneSize(-1, 100)
	if err == nil {
		t.Error("expected error for negative width, got nil")
	}
}

func TestNewSceneSize_ZeroHeight(t *testing.T) {
	_, err := NewSceneSize(100, 0)
	if err == nil {
		t.Error("expected error for zero height, got nil")
	}
}

func TestNewSceneSize_NegativeHeight(t *testing.T) {
	_, err := NewSceneSize(100, -5)
	if err == nil {
		t.Error("expected error for negative height, got nil")
	}
}

func TestNewSceneSize_BothNonPositive(t *testing.T) {
	_, err := NewSceneSize(0, 0)
	if err == nil {
		t.Error("expected error for zero width and height, got nil")
	}
}

func TestSceneSize_String(t *testing.T) {
	s, _ := NewSceneSize(800, 600)
	str := s.String()
	if !strings.Contains(str, "800") {
		t.Errorf("String() should contain width 800, got %q", str)
	}
	if !strings.Contains(str, "600") {
		t.Errorf("String() should contain height 600, got %q", str)
	}
}

func TestNewSceneSize_MinimalPositive(t *testing.T) {
	s, err := NewSceneSize(1, 1)
	if err != nil {
		t.Fatalf("unexpected error for 1x1 size: %v", err)
	}
	if s.Width() != 1 || s.Height() != 1 {
		t.Errorf("expected 1x1, got %dx%d", s.Width(), s.Height())
	}
}
