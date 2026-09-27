package tag

import (
	"testing"
)

func TestNewTagValue_DispatchesByType(t *testing.T) {
	cases := []struct {
		name         string
		tagType      TagType
		input        any
		expectedType string
	}{
		{"string tagType", TagTypeString, "hello", "TagValueString"},
		{"boolean tagType", TagTypeBoolean, true, "TagValueBoolean"},
		{"integer tagType", TagTypeInteger, 42, "TagValueInteger"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := tc.tagType.NewTagValue(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v == nil {
				t.Fatal("expected non-nil TagValue")
			}
		})
	}
}

func TestNewTagValue_StringType_StoresCorrectValue(t *testing.T) {
	v, err := TagTypeString.NewTagValue("world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.String() != "world" {
		t.Errorf("expected 'world', got %q", v.String())
	}
}

func TestNewTagValue_BooleanType_StoresCorrectValue(t *testing.T) {
	v, err := TagTypeBoolean.NewTagValue(true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.String() != "true" {
		t.Errorf("expected 'true', got %q", v.String())
	}
}

func TestNewTagValue_IntegerType_StoresCorrectValue(t *testing.T) {
	v, err := TagTypeInteger.NewTagValue(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.String() != "7" {
		t.Errorf("expected '7', got %q", v.String())
	}
}

func TestNewTagValue_IncompatibleInput_ReturnsError(t *testing.T) {
	// float64 is not supported by any value constructor
	_, err := TagTypeString.NewTagValue(3.14)
	if err == nil {
		t.Error("expected error for unsupported input type, got nil")
	}
}
