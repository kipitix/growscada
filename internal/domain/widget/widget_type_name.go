package widget

import "fmt"

// WidgetTypeName - name of the widget type.
// Value Object.
type WidgetTypeName struct {
	name string
}

var _ fmt.Stringer = WidgetTypeName{}

// NewWidgetTypeName creates a WidgetTypeName from a string.
func NewWidgetTypeName(aName string) (WidgetTypeName, error) {
	// TODO: add restrictions and validation
	return WidgetTypeName{name: aName}, nil
}

// String implements [fmt.Stringer].
func (n WidgetTypeName) String() string {
	return n.name
}
