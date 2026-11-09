package description

import (
	"strings"
	"testing"
)

func TestNew_TrimsAndAcceptsEmpty(t *testing.T) {
	d, err := New("  A line of the plant  ")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if d.String() != "A line of the plant" || d.IsEmpty() {
		t.Errorf("got %q, want the trimmed text", d.String())
	}

	empty, err := New("   ")
	if err != nil {
		t.Fatalf("New(empty): %v", err)
	}
	if !empty.IsEmpty() || empty.String() != "" {
		t.Errorf("got %q, want an empty description", empty.String())
	}
}

func TestNew_RefusesMoreThanMaxLengthCharacters(t *testing.T) {
	if _, err := New(strings.Repeat("я", MaxLength)); err != nil {
		t.Errorf("a description of %d characters must pass: %v", MaxLength, err)
	}
	if _, err := New(strings.Repeat("я", MaxLength+1)); err == nil {
		t.Error("expected an error for one character too many")
	}
}

func TestFromStorage_KeepsWhatWasStored(t *testing.T) {
	if got := FromStorage("  stored  ").String(); got != "stored" {
		t.Errorf("got %q, want %q", got, "stored")
	}
	if got := FromStorage(strings.Repeat("x", MaxLength+10)).String(); len(got) != MaxLength+10 {
		t.Errorf("FromStorage must not apply the length check")
	}
}
