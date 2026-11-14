package label

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// NameMaxLength is the longest label name, in characters (runes) after
// trimming.
const NameMaxLength = 100

// Name - the name of a Label: trimmed, 1–NameMaxLength characters, no line
// breaks. It is unique in the Organization without regard to case (`Prod` and
// `prod` are one name); the repository enforces that.
// Value Object.
type Name struct {
	name string
}

var _ fmt.Stringer = Name{}

// NewName creates a Name from a string: the text is trimmed of its
// surrounding space, must not be empty, must be at most NameMaxLength
// characters and must hold no line break.
func NewName(aName string) (Name, error) {
	name := Name{name: strings.TrimSpace(aName)}
	if err := name.validate(); err != nil {
		return Name{}, err
	}
	return name, nil
}

// validate reports whether the name holds a value NewName accepts. The zero
// Name is invalid: a label always has a name.
func (n Name) validate() error {
	switch {
	case n.name == "":
		return fmt.Errorf("%w: must not be empty", ErrInvalidLabelName)
	case utf8.RuneCountInString(n.name) > NameMaxLength:
		return fmt.Errorf("%w: must be at most %d characters, got %d",
			ErrInvalidLabelName, NameMaxLength, utf8.RuneCountInString(n.name))
	case strings.ContainsAny(n.name, "\r\n"):
		return fmt.Errorf("%w: must be one line", ErrInvalidLabelName)
	}
	return nil
}

// String implements [fmt.Stringer].
func (n Name) String() string { return n.name }
