package version

import "fmt"

// Version is a value object representing an optimistic-lock version number.
// The type parameter T is a phantom type that binds the version to a specific aggregate,
// preventing accidental cross-aggregate version comparisons.
type Version[T any] struct {
	number int
}

var _ fmt.Stringer = Version[struct{}]{}

// Initial returns the zero version assigned to a newly created aggregate (not yet persisted).
func Initial[T any]() Version[T] { return Version[T]{number: 0} }

// Committed returns the first persisted version.
func Committed[T any]() Version[T] { return Version[T]{number: 1} }

// Option is a functional option for Version construction.
type Option[T any] func(*Version[T])

// New creates a Version with the given options.
func New[T any](opts ...Option[T]) (Version[T], error) {
	v := Version[T]{number: 0}

	for _, opt := range opts {
		opt(&v)
	}

	if v.number < 0 {
		return Version[T]{}, fmt.Errorf("version cannot be negative: %d", v.number)
	}

	return v, nil
}

// WithNumber sets the version number.
func WithNumber[T any](number int) Option[T] {
	return func(v *Version[T]) {
		v.number = number
	}
}

// Number returns the underlying integer value.
func (v Version[T]) Number() int { return v.number }

// Next returns the incremented version.
func (v Version[T]) Next() Version[T] { return Version[T]{number: v.number + 1} }

// IsCommitted returns true if the version is greater than zero.
func (v Version[T]) IsCommitted() bool { return v.number > 0 }

// String implements [fmt.Stringer].
func (v Version[T]) String() string { return fmt.Sprintf("%d", v.number) }
