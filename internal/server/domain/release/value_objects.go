package release

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// noteMaxRunes is the longest note a Release takes.
const noteMaxRunes = 2000

// Number - the number of a Project Release within its Project: 1, 2, 3… It
// is the Release's identity within the scope; a Library Release has its own
// number, in the library package (task 72_5). The zero value is no Release.
// Value Object.
type Number struct {
	n int
}

var _ fmt.Stringer = Number{}

// NewNumber creates a Number; it must be positive.
func NewNumber(n int) (Number, error) {
	if n <= 0 {
		return Number{}, fmt.Errorf("release number must be positive, got %d", n)
	}
	return Number{n: n}, nil
}

// Int returns the number; 0 for no Release.
func (n Number) Int() int { return n.n }

// IsValid reports whether n is a Release's number, not the zero value.
func (n Number) IsValid() bool { return n.n > 0 }

// String implements [fmt.Stringer].
func (n Number) String() string { return strconv.Itoa(n.n) }

// Note - the optional note of a Release: a trimmed line for people, empty for
// none. Unlike the Release's content it is metadata and changes on its own
// (Release.WithNote).
// Value Object.
type Note struct {
	note string
}

var _ fmt.Stringer = Note{}

// NewNote creates a Note from a string: the surrounding spaces are dropped,
// the rest may be empty and is at most 2000 characters.
func NewNote(aNote string) (Note, error) {
	note := strings.TrimSpace(aNote)
	if utf8.RuneCountInString(note) > noteMaxRunes {
		return Note{}, fmt.Errorf("release note must be at most %d characters", noteMaxRunes)
	}
	return Note{note: note}, nil
}

// String implements [fmt.Stringer].
func (n Note) String() string { return n.note }

// Origin - where a Release's content came from: a Commit, or a project file
// (task 72_4). The zero value is invalid (ADR 0001).
// Value Object.
type Origin struct {
	origin int
}

var _ fmt.Stringer = Origin{}

// OriginCommit - a Release made of a Commit.
// OriginFile - a Release made of a project file, without a Commit.
var (
	OriginCommit = Origin{origin: 1}
	OriginFile   = Origin{origin: 2}
)

// NewOrigin parses a string into the enum.
// Returns an error for any string that is not a known origin.
func NewOrigin(s string) (Origin, error) {
	switch s {
	case "commit":
		return OriginCommit, nil
	case "file":
		return OriginFile, nil
	default:
		return Origin{}, fmt.Errorf("unknown release origin: %q", s)
	}
}

// IsValid reports whether the origin is one of the known values (not the zero
// value).
func (o Origin) IsValid() bool { return o != Origin{} }

// String returns the string representation of the origin.
func (o Origin) String() string {
	switch o {
	case OriginCommit:
		return "commit"
	case OriginFile:
		return "file"
	default:
		return "invalid"
	}
}
