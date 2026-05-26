package widget

import (
	"strings"
	"testing"
)

func TestNewSize_Valid(t *testing.T) {
	s, err := NewSize(200, 150)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Width() != 200 {
		t.Errorf("Width: expected 200, got %d", s.Width())
	}
	if s.Height() != 150 {
		t.Errorf("Height: expected 150, got %d", s.Height())
	}
}

func TestNewSize_ZeroWidth(t *testing.T) {
	_, err := NewSize(0, 100)
	if err == nil {
		t.Error("expected error for zero width, got nil")
	}
}

func TestNewSize_NegativeWidth(t *testing.T) {
	_, err := NewSize(-10, 100)
	if err == nil {
		t.Error("expected error for negative width, got nil")
	}
}

func TestNewSize_ZeroHeight(t *testing.T) {
	_, err := NewSize(100, 0)
	if err == nil {
		t.Error("expected error for zero height, got nil")
	}
}

func TestNewSize_NegativeHeight(t *testing.T) {
	_, err := NewSize(100, -5)
	if err == nil {
		t.Error("expected error for negative height, got nil")
	}
}

func TestNewSize_BothNonPositive(t *testing.T) {
	_, err := NewSize(0, 0)
	if err == nil {
		t.Error("expected error for zero width and height, got nil")
	}
}

func TestDefaultSize(t *testing.T) {
	s := DefaultSize()
	if s.Width() != 100 {
		t.Errorf("DefaultSize Width: expected 100, got %d", s.Width())
	}
	if s.Height() != 100 {
		t.Errorf("DefaultSize Height: expected 100, got %d", s.Height())
	}
}

func TestSize_String(t *testing.T) {
	s, _ := NewSize(640, 480)
	str := s.String()
	if !strings.Contains(str, "640") {
		t.Errorf("String() should contain width 640, got %q", str)
	}
	if !strings.Contains(str, "480") {
		t.Errorf("String() should contain height 480, got %q", str)
	}
}
