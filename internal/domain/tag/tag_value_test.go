package tag

import (
	"testing"
)

func TestNewTagValue_DispatchesByKind(t *testing.T) {
	cases := []struct {
		name         string
		kind         TagKind
		input        any
		expectedType string
	}{
		{"string kind", TagKindString, "hello", "TagValueString"},
		{"boolean kind", TagKindBoolean, true, "TagValueBoolean"},
		{"integer kind", TagKindInteger, 42, "TagValueInteger"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := tc.kind.NewTagValue(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v == nil {
				t.Fatal("expected non-nil TagValue")
			}
		})
	}
}

func TestNewTagValue_StringKind_StoresCorrectValue(t *testing.T) {
	v, err := TagKindString.NewTagValue("world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.String() != "world" {
		t.Errorf("expected 'world', got %q", v.String())
	}
}

func TestNewTagValue_BooleanKind_StoresCorrectValue(t *testing.T) {
	v, err := TagKindBoolean.NewTagValue(true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.String() != "true" {
		t.Errorf("expected 'true', got %q", v.String())
	}
}

func TestNewTagValue_IntegerKind_StoresCorrectValue(t *testing.T) {
	v, err := TagKindInteger.NewTagValue(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.String() != "7" {
		t.Errorf("expected '7', got %q", v.String())
	}
}

func TestNewTagValue_UnknownKind_ReturnsError(t *testing.T) {
	_, err := TagKind(99).NewTagValue("x")
	if err == nil {
		t.Error("expected error for unknown kind, got nil")
	}
}

func TestNewTagValue_IncompatibleInput_ReturnsError(t *testing.T) {
	// float64 is not supported by any value constructor
	_, err := TagKindString.NewTagValue(3.14)
	if err == nil {
		t.Error("expected error for unsupported input type, got nil")
	}
}
