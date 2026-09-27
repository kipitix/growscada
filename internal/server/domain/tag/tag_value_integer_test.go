package tag

import (
	"math"
	"testing"
)

func TestNewTagValueInteger_FromString(t *testing.T) {
	cases := []struct {
		input    string
		expected int64
		wantErr  bool
	}{
		{"42", 42, false},
		{"0", 0, false},
		{"-7", -7, false},
		{"9223372036854775807", math.MaxInt64, false}, // int64 max
		{"abc", 0, true},
		{"3.14", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			v, err := NewTagValueInteger(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if int64(v) != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, int64(v))
			}
		})
	}
}

func TestNewTagValueInteger_FromBool(t *testing.T) {
	v, err := NewTagValueInteger(true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int64(v) != 1 {
		t.Errorf("expected 1 for true, got %d", int64(v))
	}

	v, err = NewTagValueInteger(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int64(v) != 0 {
		t.Errorf("expected 0 for false, got %d", int64(v))
	}
}

func TestNewTagValueInteger_FromInt(t *testing.T) {
	v, err := NewTagValueInteger(int(100))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int64(v) != 100 {
		t.Errorf("expected 100, got %d", int64(v))
	}
}

func TestNewTagValueInteger_FromInt32(t *testing.T) {
	v, err := NewTagValueInteger(int32(math.MaxInt32))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int64(v) != math.MaxInt32 {
		t.Errorf("expected %d, got %d", int64(math.MaxInt32), int64(v))
	}
}

func TestNewTagValueInteger_FromInt64(t *testing.T) {
	large := int64(math.MaxInt64)
	v, err := NewTagValueInteger(large)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if int64(v) != large {
		t.Errorf("expected %d, got %d", large, int64(v))
	}
}

func TestNewTagValueInteger_UnsupportedType(t *testing.T) {
	_, err := NewTagValueInteger(3.14)
	if err == nil {
		t.Error("expected error for float64 input, got nil")
	}
}

func TestTagValueInteger_Value(t *testing.T) {
	v := TagValueInteger(55)
	got, ok := v.Value().(int64)
	if !ok {
		t.Fatal("Value() did not return int64")
	}
	if got != 55 {
		t.Errorf("expected 55, got %d", got)
	}
}

func TestTagValueInteger_String(t *testing.T) {
	cases := []struct {
		input    TagValueInteger
		expected string
	}{
		{0, "0"},
		{42, "42"},
		{-7, "-7"},
	}

	for _, tc := range cases {
		if got := tc.input.String(); got != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, got)
		}
	}
}
