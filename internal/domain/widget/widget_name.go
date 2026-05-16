package widget

import "fmt"

// WidgetName - name of a widget instance.
// Value Object.
type WidgetName struct {
	name string
}

var _ fmt.Stringer = WidgetName{}

// NewWidgetName creates a WidgetName from a string.
func NewWidgetName(aName string) (WidgetName, error) {
	if aName == "" {
		return WidgetName{}, fmt.Errorf("widget name must not be empty")
	}
	return WidgetName{name: aName}, nil
}

// String implements [fmt.Stringer].
func (n WidgetName) String() string {
	return n.name
}
