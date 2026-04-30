package tag

import "testing"

func TestNewTagValueString_FromString(t *testing.T) {
	v, err := NewTagValueString("hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(v) != "hello" {
		t.Errorf("expected 'hello', got %q", string(v))
	}
}

func TestNewTagValueString_FromBool(t *testing.T) {
	cases := []struct {
		input    bool
		expected string
	}{
		{true, "true"},
		{false, "false"},
	}

	for _, tc := range cases {
		v, err := NewTagValueString(tc.input)
		if err != nil {
			t.Fatalf("unexpected error for %v: %v", tc.input, err)
		}
		if string(v) != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, string(v))
		}
	}
}

func TestNewTagValueString_FromInt(t *testing.T) {
	v, err := NewTagValueString(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(v) != "42" {
		t.Errorf("expected '42', got %q", string(v))
	}
}

func TestNewTagValueString_UnsupportedType(t *testing.T) {
	_, err := NewTagValueString(3.14)
	if err == nil {
		t.Error("expected error for float64 input, got nil")
	}
}

func TestTagValueString_Value(t *testing.T) {
	v := TagValueString("sensor")
	got, ok := v.Value().(string)
	if !ok {
		t.Fatal("Value() did not return string")
	}
	if got != "sensor" {
		t.Errorf("expected 'sensor', got %q", got)
	}
}

func TestTagValueString_String(t *testing.T) {
	v := TagValueString("test_value")
	if v.String() != "test_value" {
		t.Errorf("expected 'test_value', got %q", v.String())
	}
}
