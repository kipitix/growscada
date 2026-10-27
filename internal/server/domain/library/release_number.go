package library

import (
	"fmt"
	"strconv"
)

// ReleaseNumber - the number of a Library Release within its Library: 1, 2,
// 3… The zero value is no release.
// Value Object.
type ReleaseNumber struct {
	n int
}

var _ fmt.Stringer = ReleaseNumber{}

// NewReleaseNumber creates a ReleaseNumber; it must be positive.
func NewReleaseNumber(n int) (ReleaseNumber, error) {
	if n <= 0 {
		return ReleaseNumber{}, fmt.Errorf("release number must be positive, got %d", n)
	}
	return ReleaseNumber{n: n}, nil
}

// FirstRelease is the number of a Library's first release.
func FirstRelease() ReleaseNumber { return ReleaseNumber{n: 1} }

// Int returns the number; 0 for no release.
func (r ReleaseNumber) Int() int { return r.n }

// IsValid reports whether r is a release's number, not the zero value.
func (r ReleaseNumber) IsValid() bool { return r.n > 0 }

// Next returns the number of the release after r: 1 after no release.
func (r ReleaseNumber) Next() ReleaseNumber { return ReleaseNumber{n: r.n + 1} }

// String implements [fmt.Stringer].
func (r ReleaseNumber) String() string { return strconv.Itoa(r.n) }
