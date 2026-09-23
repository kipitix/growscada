package widget

import (
	"testing"

	"github.com/kipitix/growscada/internal/domain/tag"
)

func TestAnyTagType_AcceptsEveryTagType(t *testing.T) {
	h := AnyTagType()
	if !h.IsAny() {
		t.Error("expected IsAny to be true")
	}
	if _, ok := h.TagType(); ok {
		t.Error("expected TagType to report no concrete type")
	}
	for _, tt := range []tag.TagType{tag.TagTypeString, tag.TagTypeBoolean, tag.TagTypeInteger} {
		if !h.Accepts(tt) {
			t.Errorf("expected any-type hint to accept %v", tt)
		}
	}
	if h.String() != "" {
		t.Errorf("expected empty string, got %q", h.String())
	}
}

func TestOnlyTagType_AcceptsOnlyThatType(t *testing.T) {
	h, err := OnlyTagType(tag.TagTypeInteger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.IsAny() {
		t.Error("expected IsAny to be false")
	}
	if got, ok := h.TagType(); !ok || got != tag.TagTypeInteger {
		t.Errorf("expected integer, got %v (ok=%v)", got, ok)
	}
	if !h.Accepts(tag.TagTypeInteger) {
		t.Error("expected hint to accept integer")
	}
	if h.Accepts(tag.TagTypeString) {
		t.Error("expected hint to reject string")
	}
	if h.String() != "integer" {
		t.Errorf("expected %q, got %q", "integer", h.String())
	}
}

func TestOnlyTagType_InvalidTagType_ReturnsError(t *testing.T) {
	if _, err := OnlyTagType(tag.TagType{}); err == nil {
		t.Error("expected error for invalid tag type, got nil")
	}
}

func TestNewPortTypeHint(t *testing.T) {
	cases := []struct {
		input   string
		wantErr bool
		want    string
	}{
		{"", false, ""},
		{"string", false, "string"},
		{"boolean", false, "boolean"},
		{"integer", false, "integer"},
		{"unknown", true, ""},
		{"float", true, ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			h, err := NewPortTypeHint(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NewPortTypeHint(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
			if !tc.wantErr && h.String() != tc.want {
				t.Errorf("expected %q, got %q", tc.want, h.String())
			}
		})
	}
}
