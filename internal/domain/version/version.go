package version

import "fmt"

// Version is a value object representing an optimistic-lock version number.
type Version struct {
	number int
}

var _ fmt.Stringer = Version{}

var (
	// Initial is the zero version assigned to a newly created aggregate (not yet persisted).
	Initial = Version{number: 0}
	// Committed is the first persisted version.
	Committed = Version{number: 1}
)

// New creates a Version with the given options.
func New(opts ...Option) (Version, error) {
	v := Version{number: 0}

	for _, opt := range opts {
		opt(&v)
	}

	if v.number < 0 {
		return Version{}, fmt.Errorf("version cannot be negative: %d", v.number)
	}

	return v, nil
}

// Option is a functional option for Version construction.
type Option func(*Version)

// WithNumber sets the version number.
func WithNumber(number int) Option {
	return func(v *Version) {
		v.number = number
	}
}

// Number returns the underlying integer value.
func (v Version) Number() int {
	return v.number
}

// Next returns the incremented version.
func (v Version) Next() Version {
	return Version{number: v.number + 1}
}

// IsCommitted returns true if the version is greater than zero.
func (v Version) IsCommitted() bool {
	return v.number > 0
}

// String implements [fmt.Stringer].
func (v Version) String() string {
	return fmt.Sprintf("%d", v.number)
}
