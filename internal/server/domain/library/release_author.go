package library

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// releaseAuthorMaxRunes is the longest Author name a Library Release takes.
const releaseAuthorMaxRunes = 100

// Author - the name an Engineer declares when publishing a Library Release
// (task 72_5). A declared name, not an account: Users come with tasks 49/50.
// Value Object.
type Author struct {
	name string
}

var _ fmt.Stringer = Author{}

// NewAuthor creates an Author from a string: the surrounding spaces are
// dropped, the rest must be non-empty and at most 100 characters.
func NewAuthor(aName string) (Author, error) {
	name := strings.TrimSpace(aName)
	if name == "" {
		return Author{}, fmt.Errorf("author must not be empty")
	}
	if utf8.RuneCountInString(name) > releaseAuthorMaxRunes {
		return Author{}, fmt.Errorf("author must be at most %d characters", releaseAuthorMaxRunes)
	}
	return Author{name: name}, nil
}

// String implements [fmt.Stringer].
func (a Author) String() string { return a.name }
