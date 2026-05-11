package indicator_type

import "fmt"

// IndicatorTypeVersion is a value object representing the optimistic-lock version of an indicator type.
type IndicatorTypeVersion int

var _ fmt.Stringer = IndicatorTypeVersion(0)

// NewIndicatorTypeVersion creates an IndicatorTypeVersion from a non-negative integer.
func NewIndicatorTypeVersion(v int) (IndicatorTypeVersion, error) {
	if v < 0 {
		return 0, fmt.Errorf("indicator type version cannot be negative: %d", v)
	}
	return IndicatorTypeVersion(v), nil
}

// Int returns the underlying integer value.
func (v IndicatorTypeVersion) Int() int {
	return int(v)
}

// String implements fmt.Stringer.
func (v IndicatorTypeVersion) String() string {
	return fmt.Sprintf("%d", int(v))
}

// Next returns the incremented version.
func (v IndicatorTypeVersion) Next() IndicatorTypeVersion {
	return v + 1
}
