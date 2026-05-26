package widget

import "testing"

// TestNewCoordinates verifies the deprecated NewCoordinates wrapper delegates
// correctly to NewPosition, truncating the float64 z to int.
func TestNewCoordinates_DelegatesToNewPosition(t *testing.T) {
	p := NewCoordinates(12.5, 34.75, 2.9)

	if p.X() != 12.5 {
		t.Errorf("X: expected 12.5, got %.4f", p.X())
	}
	if p.Y() != 34.75 {
		t.Errorf("Y: expected 34.75, got %.4f", p.Y())
	}
	// float64 → int truncation: 2.9 → 2
	if p.Z() != 2 {
		t.Errorf("Z: expected 2 (truncated from 2.9), got %d", p.Z())
	}
}

func TestNewCoordinates_ZeroValues(t *testing.T) {
	p := NewCoordinates(0, 0, 0)
	if p.X() != 0 || p.Y() != 0 || p.Z() != 0 {
		t.Errorf("expected (0, 0, z:0), got (%.4f, %.4f, z:%d)", p.X(), p.Y(), p.Z())
	}
}
