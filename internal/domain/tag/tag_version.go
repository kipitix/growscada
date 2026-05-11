package tag

import "fmt"

// TagVersion is a value object representing the optimistic-lock version of a tag.
type TagVersion int

var _ fmt.Stringer = TagVersion(0)

// NewTagVersion creates a TagVersion from a non-negative integer.
func NewTagVersion(v int) (TagVersion, error) {
	if v < 0 {
		return 0, fmt.Errorf("tag version cannot be negative: %d", v)
	}
	return TagVersion(v), nil
}

// Int returns the underlying integer value.
func (v TagVersion) Int() int {
	return int(v)
}

// String implements fmt.Stringer.
func (v TagVersion) String() string {
	return fmt.Sprintf("%d", int(v))
}

// Next returns the incremented version.
func (v TagVersion) Next() TagVersion {
	return v + 1
}
