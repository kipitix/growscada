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

func TestNewTagType_UnrecognisedValue_ReturnsErrorAndInvalid(t *testing.T) {
	for _, input := range []string{"float", "unknown", ""} {
		t.Run(input, func(t *testing.T) {
			tagType, err := NewTagType(input)
			if err == nil {
				t.Errorf("expected error for %q, got nil", input)
			}
			if tagType.IsValid() {
				t.Errorf("expected invalid zero value for %q, got %v", input, tagType)
			}
		})
	}
}

func TestTagType_IsValid(t *testing.T) {
	for _, tt := range []TagType{TagTypeString, TagTypeBoolean, TagTypeInteger} {
		if !tt.IsValid() {
			t.Errorf("expected %v to be valid", tt)
		}
	}
	if (TagType{}).IsValid() {
		t.Error("expected zero value to be invalid")
	}
}

func TestTagType_String(t *testing.T) {
	cases := []struct {
		name     string
		tagType  TagType
		expected string
	}{
		{"zero", TagType{}, "invalid"},
		{"string", TagTypeString, "string"},
		{"boolean", TagTypeBoolean, "boolean"},
		{"integer", TagTypeInteger, "integer"},
		{"default", TagType{tagType: 99}, "invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tagType.String(); got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
