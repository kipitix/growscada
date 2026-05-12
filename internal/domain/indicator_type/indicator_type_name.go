package indicator_type

import "fmt"

// IndicatorTypeName - name of the indicator type.
// Value Object.
type IndicatorTypeName struct {
	name string
}

// Interfaces for IndicatorTypeName.
var _ fmt.Stringer = IndicatorTypeName{}

// Factory method.
func NewIndicatorTypeName(aName string) (IndicatorTypeName, error) {
	// TODO: add restrictions and validation
	return IndicatorTypeName{name: aName}, nil
}

// String method.
func (n IndicatorTypeName) String() string {
	return n.name
}
