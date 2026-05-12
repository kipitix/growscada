package tag

import "testing"

func TestNewTagType_ValidValues(t *testing.T) {
	cases := []struct {
		input    string
		expected TagType
	}{
		{"string", TagTypeString},
		{"boolean", TagTypeBoolean},
		{"integer", TagTypeInteger},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tagType, err := NewTagType(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if tagType != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, tagType)
			}
		})
	}
}

func TestNewTagType_UnknownValue(t *testing.T) {
	_, err := NewTagType("float")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}

func TestTagType_String(t *testing.T) {
	cases := []struct {
		tagType  TagType
		expected string
	}{
		{TagTypeUnknown, "unknown"},
		{TagTypeString, "string"},
		{TagTypeBoolean, "boolean"},
		{TagTypeInteger, "integer"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			if got := tc.tagType.String(); got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
