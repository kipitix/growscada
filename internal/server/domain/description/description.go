// Package description holds the Description of a Project or a Library: an
// optional, free-text attribute of the aggregate itself. It is never part of
// the aggregate's content: a ProjectFile, a Deployment, a Commit or a growctl
// manifest does not carry it (task 73).
package description

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxLength is the longest Description, in characters (runes) after trimming.
const MaxLength = 500

// Description - optional description of a Project or a Library.
// Value Object.
type Description struct {
	text string
}

var _ fmt.Stringer = Description{}

// New creates a Description from a string: the text is trimmed of its
// surrounding space and at most MaxLength characters long. An empty text is a
// valid Description — the entity simply has none.
func New(aText string) (Description, error) {
	trimmed := strings.TrimSpace(aText)
	if n := utf8.RuneCountInString(trimmed); n > MaxLength {
		return Description{}, fmt.Errorf("description must be at most %d characters, got %d", MaxLength, n)
	}
	return Description{text: trimmed}, nil
}

// FromStorage rebuilds a Description as stored, skipping the length check: the
// value was validated when it was written. For repositories.
func FromStorage(aText string) Description {
	return Description{text: strings.TrimSpace(aText)}
}

// IsEmpty reports whether the entity has no description.
func (d Description) IsEmpty() bool { return d.text == "" }

// String implements [fmt.Stringer].
func (d Description) String() string { return d.text }
