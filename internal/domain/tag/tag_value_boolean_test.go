package tag

import "testing"

func TestNewTagValueBoolean_FromString(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
		wantErr  bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"1", false, true},
		{"yes", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			v, err := NewTagValueBoolean(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bool(v) != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, bool(v))
			}
		})
	}
}

func TestNewTagValueBoolean_FromBool(t *testing.T) {
	for _, b := range []bool{true, false} {
		v, err := NewTagValueBoolean(b)
		if err != nil {
			t.Fatalf("unexpected error for %v: %v", b, err)
		}
		if bool(v) != b {
			t.Errorf("expected %v, got %v", b, bool(v))
		}
	}
}

func TestNewTagValueBoolean_FromInt(t *testing.T) {
	cases := []struct {
		input    int
		expected bool
	}{
		{0, false},
		{1, true},
		{-1, true},
		{42, true},
	}

	for _, tc := range cases {
		v, err := NewTagValueBoolean(tc.input)
		if err != nil {
			t.Fatalf("unexpected error for %v: %v", tc.input, err)
		}
		if bool(v) != tc.expected {
			t.Errorf("input %d: expected %v, got %v", tc.input, tc.expected, bool(v))
		}
	}
}

func TestNewTagValueBoolean_UnsupportedType(t *testing.T) {
	_, err := NewTagValueBoolean(3.14)
	if err == nil {
		t.Error("expected error for float64 input, got nil")
	}
}

func TestTagValueBoolean_Value(t *testing.T) {
	v := TagValueBoolean(true)
	got, ok := v.Value().(bool)
	if !ok {
		t.Fatal("Value() did not return bool")
	}
	if !got {
		t.Error("expected true, got false")
	}
}

func TestTagValueBoolean_String(t *testing.T) {
	if TagValueBoolean(true).String() != "true" {
		t.Error("expected 'true'")
	}
	if TagValueBoolean(false).String() != "false" {
		t.Error("expected 'false'")
	}
}
