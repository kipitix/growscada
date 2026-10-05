package scene

import (
	"strings"
	"testing"
)

func TestNewWidgetSize_Valid(t *testing.T) {
	s, err := NewWidgetSize(200, 150)
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

func TestNewWidgetSize_ZeroWidth(t *testing.T) {
	_, err := NewWidgetSize(0, 100)
	if err == nil {
		t.Error("expected error for zero width, got nil")
	}
}

func TestNewWidgetSize_NegativeWidth(t *testing.T) {
	_, err := NewWidgetSize(-10, 100)
	if err == nil {
		t.Error("expected error for negative width, got nil")
	}
}

func TestNewWidgetSize_ZeroHeight(t *testing.T) {
	_, err := NewWidgetSize(100, 0)
	if err == nil {
		t.Error("expected error for zero height, got nil")
	}
}

func TestNewWidgetSize_NegativeHeight(t *testing.T) {
	_, err := NewWidgetSize(100, -5)
	if err == nil {
		t.Error("expected error for negative height, got nil")
	}
}

func TestNewWidgetSize_BothNonPositive(t *testing.T) {
	_, err := NewWidgetSize(0, 0)
	if err == nil {
		t.Error("expected error for zero width and height, got nil")
	}
}

func TestDefaultWidgetSize(t *testing.T) {
	s := DefaultWidgetSize()
	if s.Width() != 100 {
		t.Errorf("DefaultWidgetSize Width: expected 100, got %d", s.Width())
	}
	if s.Height() != 100 {
		t.Errorf("DefaultWidgetSize Height: expected 100, got %d", s.Height())
	}
}

func TestWidgetSize_String(t *testing.T) {
	s, _ := NewWidgetSize(640, 480)
	str := s.String()
	if !strings.Contains(str, "640") {
		t.Errorf("String() should contain width 640, got %q", str)
	}
	if !strings.Contains(str, "480") {
		t.Errorf("String() should contain height 480, got %q", str)
	}
}
