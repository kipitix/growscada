package tag

import "testing"

func TestNewTagKind_ValidValues(t *testing.T) {
	cases := []struct {
		input    string
		expected TagKind
	}{
		{"string", TagKindString},
		{"boolean", TagKindBoolean},
		{"integer", TagKindInteger},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			kind, err := NewTagKind(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if kind != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, kind)
			}
		})
	}
}

func TestNewTagKind_UnknownValue(t *testing.T) {
	_, err := NewTagKind("float")
	if err == nil {
		t.Error("expected error for unknown kind, got nil")
	}
}

func TestTagKind_String(t *testing.T) {
	cases := []struct {
		kind     TagKind
		expected string
	}{
		{TagKindString, "string"},
		{TagKindBoolean, "boolean"},
		{TagKindInteger, "integer"},
		{TagKind(-1), "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			if got := tc.kind.String(); got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestTagKind_IsValid(t *testing.T) {
	valid := []TagKind{TagKindString, TagKindBoolean, TagKindInteger}
	for _, k := range valid {
		if !k.IsValid() {
			t.Errorf("expected %v to be valid", k)
		}
	}

	invalid := []TagKind{TagKind(-1), TagKind(99)}
	for _, k := range invalid {
		if k.IsValid() {
			t.Errorf("expected %v to be invalid", k)
		}
	}
}
