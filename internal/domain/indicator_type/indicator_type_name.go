package indicator_type

import "fmt"

// IndicatorTypeName - name of the indicator type.
type IndicatorTypeName string

// Interfaces for IndicatorTypeName.
var _ fmt.Stringer = IndicatorTypeName("")

// NewIndicatorTypeName creates an IndicatorTypeName from a string.
func NewIndicatorTypeName(name string) (IndicatorTypeName, error) {
	return IndicatorTypeName(name), nil
}

// String implements [fmt.Stringer].
func (n IndicatorTypeName) String() string {
	return string(n)
}
