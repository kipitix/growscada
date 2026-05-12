package indicator_type

import "fmt"

// IndicatorTypeVersion is a value object representing the optimistic-lock version of an indicator type.
type IndicatorTypeVersion struct {
	number int
}

var _ fmt.Stringer = IndicatorTypeVersion{}

var (
	IndicatorTypeVersionInitial   = IndicatorTypeVersion{number: 0}
	IndicatorTypeVersionCommitted = IndicatorTypeVersion{number: 1}
)

func NewIndicatorTypeVersion(opts ...IndicatorTypeVersionOption) (IndicatorTypeVersion, error) {
	version := IndicatorTypeVersion{number: 0}

	// Apply options
	for _, opt := range opts {
		opt(&version)
	}

	// Validate
	if version.number < 0 {
		return IndicatorTypeVersion{}, fmt.Errorf("indicator type version cannot be negative: %d", version.number)
	}

	return version, nil
}

type IndicatorTypeVersionOption func(*IndicatorTypeVersion)

func IndicatorTypeVersionWithNumber(number int) IndicatorTypeVersionOption {
	return func(v *IndicatorTypeVersion) {
		v.number = number
	}
}

// Number returns the underlying integer value.
func (v IndicatorTypeVersion) Number() int {
	return v.number
}

// Next returns the incremented version.
func (v IndicatorTypeVersion) Next() IndicatorTypeVersion {
	return IndicatorTypeVersion{number: v.number + 1}
}

// IsCommitted returns true if the version is greater than zero.
func (v IndicatorTypeVersion) IsCommitted() bool {
	return v.number > 0
}

// String implements fmt.Stringer.
func (v IndicatorTypeVersion) String() string {
	return fmt.Sprintf("%d", int(v.number))
}
