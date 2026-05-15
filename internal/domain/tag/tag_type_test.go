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
		name     string
		tagType  TagType
		expected string
	}{
		{"unknown", TagTypeUnknown, "unknown"},
		{"string", TagTypeString, "string"},
		{"boolean", TagTypeBoolean, "boolean"},
		{"integer", TagTypeInteger, "integer"},
		{"default", TagType{tagType: 99}, "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tagType.String(); got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
