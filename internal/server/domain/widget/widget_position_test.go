package widget

import (
	"strings"
	"testing"
)

func TestNewPosition_Fields(t *testing.T) {
	p := NewPosition(10.5, 20.75, 3)

	if p.X() != 10.5 {
		t.Errorf("X: expected 10.5, got %.4f", p.X())
	}
	if p.Y() != 20.75 {
		t.Errorf("Y: expected 20.75, got %.4f", p.Y())
	}
	if p.Z() != 3 {
		t.Errorf("Z: expected 3, got %d", p.Z())
	}
}

func TestNewPosition_ZeroValues(t *testing.T) {
	p := NewPosition(0, 0, 0)

	if p.X() != 0 || p.Y() != 0 || p.Z() != 0 {
		t.Errorf("expected (0, 0, 0), got (%.4f, %.4f, %d)", p.X(), p.Y(), p.Z())
	}
}

func TestNewPosition_NegativeCoordinates(t *testing.T) {
	p := NewPosition(-5.5, -100, -1)

	if p.X() != -5.5 {
		t.Errorf("X: expected -5.5, got %.4f", p.X())
	}
	if p.Y() != -100 {
		t.Errorf("Y: expected -100, got %.4f", p.Y())
	}
	if p.Z() != -1 {
		t.Errorf("Z: expected -1, got %d", p.Z())
	}
}

func TestPosition_String(t *testing.T) {
	p := NewPosition(1.5, 2.5, 0)
	s := p.String()
	if s == "" {
		t.Error("expected non-empty String() output")
	}
	if !strings.Contains(s, "1.50") {
		t.Errorf("String() should contain X value, got %q", s)
	}
	if !strings.Contains(s, "2.50") {
		t.Errorf("String() should contain Y value, got %q", s)
	}
}
