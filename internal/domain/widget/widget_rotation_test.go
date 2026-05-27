package widget

import (
	"math"
	"strings"
	"testing"
)

func TestNewRotation_NormalRange(t *testing.T) {
	cases := []float64{0, 45, 90, 180, 270, 359.99}
	for _, deg := range cases {
		r := NewRotation(deg)
		if r.Degrees() != deg {
			t.Errorf("NewRotation(%.2f).Degrees() = %.4f, want %.4f", deg, r.Degrees(), deg)
		}
	}
}

func TestNewRotation_NormalizesTo360(t *testing.T) {
	cases := []struct {
		input    float64
		expected float64
	}{
		{360, 0},
		{720, 0},
		{361, 1},
		{540, 180},
		{-90, 270},
		{-180, 180},
		{-360, 0},
		{-1, 359},
	}
	for _, tc := range cases {
		r := NewRotation(tc.input)
		if math.Abs(r.Degrees()-tc.expected) > 1e-9 {
			t.Errorf("NewRotation(%.2f).Degrees() = %.4f, want %.4f", tc.input, r.Degrees(), tc.expected)
		}
	}
}

func TestDefaultRotation(t *testing.T) {
	r := DefaultRotation()
	if r.Degrees() != 0 {
		t.Errorf("DefaultRotation: expected 0, got %.4f", r.Degrees())
	}
}

func TestRotation_String(t *testing.T) {
	r := NewRotation(45)
	s := r.String()
	if !strings.Contains(s, "45") {
		t.Errorf("Rotation.String() should contain degrees value, got %q", s)
	}
}
