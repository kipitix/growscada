package draft

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// savedStateTitleMaxRunes is the longest title a saved state takes.
const savedStateTitleMaxRunes = 200

// authorMaxRunes is the longest Author name a saved state takes.
const authorMaxRunes = 100

// SavedStateTitle - the title of a saved state: a non-empty, trimmed, short
// line naming what the Save fixed. It changes on its own, without touching the
// snapshot (SavedState.WithTitle). Value Object.
type SavedStateTitle struct {
	title string
}

var _ fmt.Stringer = SavedStateTitle{}

// NewSavedStateTitle creates a SavedStateTitle from a string: the surrounding
// spaces are dropped, the rest must be non-empty and at most 200 characters.
func NewSavedStateTitle(aTitle string) (SavedStateTitle, error) {
	title := strings.TrimSpace(aTitle)
	if title == "" {
		return SavedStateTitle{}, fmt.Errorf("saved state title must not be empty")
	}
	if utf8.RuneCountInString(title) > savedStateTitleMaxRunes {
		return SavedStateTitle{}, fmt.Errorf("saved state title must be at most %d characters", savedStateTitleMaxRunes)
	}
	return SavedStateTitle{title: title}, nil
}

// String implements [fmt.Stringer].
func (t SavedStateTitle) String() string { return t.title }

// Author - the name an Engineer declares when saving a Draft (task 72_2). A
// declared name, not an account: Users come with tasks 49/50.
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
	if utf8.RuneCountInString(name) > authorMaxRunes {
		return Author{}, fmt.Errorf("author must be at most %d characters", authorMaxRunes)
	}
	return Author{name: name}, nil
}

// String implements [fmt.Stringer].
func (a Author) String() string { return a.name }
