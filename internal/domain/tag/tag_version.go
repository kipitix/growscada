package tag

import "fmt"

// TagVersion is a value object representing the optimistic-lock version of a tag.
type TagVersion struct {
	number int
}

var _ fmt.Stringer = TagVersion{}

var (
	TagVersionInitial   = TagVersion{number: 0}
	TagVersionCommitted = TagVersion{number: 1}
)

func NewTagVersion(opts ...TagVersionOption) (TagVersion, error) {
	version := TagVersion{number: 0}

	// Apply options
	for _, opt := range opts {
		opt(&version)
	}

	// Validate
	if version.number < 0 {
		return TagVersion{}, fmt.Errorf("tag version cannot be negative: %d", version.number)
	}

	return version, nil
}

type TagVersionOption func(*TagVersion)

func TagVersionWithNumber(number int) TagVersionOption {
	return func(v *TagVersion) {
		v.number = number
	}
}

// Int returns the underlying integer value.
func (v TagVersion) Number() int {
	return v.number
}

// Next returns the incremented version.
func (v TagVersion) Next() TagVersion {
	return TagVersion{number: v.number + 1}
}

func (v TagVersion) IsCommitted() bool {
	return v.number > 0
}

// String implements fmt.Stringer.
func (v TagVersion) String() string {
	return fmt.Sprintf("%d", int(v.number))
}
