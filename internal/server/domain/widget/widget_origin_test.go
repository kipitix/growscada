package widget

import "testing"

func TestNewOrigin_Valid(t *testing.T) {
	cases := []struct{ x, y float64 }{
		{0, 0},
		{1, 1},
		{0.5, 0.5},
		{0, 1},
		{1, 0},
	}
	for _, tc := range cases {
		o, err := NewOrigin(tc.x, tc.y)
		if err != nil {
			t.Errorf("NewOrigin(%.2f, %.2f) unexpected error: %v", tc.x, tc.y, err)
			continue
		}
		if o.X() != tc.x {
			t.Errorf("X: expected %.4f, got %.4f", tc.x, o.X())
		}
		if o.Y() != tc.y {
			t.Errorf("Y: expected %.4f, got %.4f", tc.y, o.Y())
		}
	}
}

func TestNewOrigin_XOutOfRange(t *testing.T) {
	cases := []float64{-0.001, 1.001, -100, 2}
	for _, x := range cases {
		_, err := NewOrigin(x, 0.5)
		if err == nil {
			t.Errorf("expected error for x=%.4f, got nil", x)
		}
	}
}

func TestNewOrigin_YOutOfRange(t *testing.T) {
	cases := []float64{-0.001, 1.001, -100, 2}
	for _, y := range cases {
		_, err := NewOrigin(0.5, y)
		if err == nil {
			t.Errorf("expected error for y=%.4f, got nil", y)
		}
	}
}

func TestDefaultOrigin(t *testing.T) {
	o := DefaultOrigin()
	if o.X() != 0.5 {
		t.Errorf("DefaultOrigin X: expected 0.5, got %.4f", o.X())
	}
	if o.Y() != 0.5 {
		t.Errorf("DefaultOrigin Y: expected 0.5, got %.4f", o.Y())
	}
}

func TestOrigin_String(t *testing.T) {
	o, _ := NewOrigin(0.25, 0.75)
	s := o.String()
	if s == "" {
		t.Error("expected non-empty String() output")
	}
}
