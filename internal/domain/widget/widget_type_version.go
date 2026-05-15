package widget

import "fmt"

// WidgetTypeVersion is a value object representing the optimistic-lock version of a widget type.
type WidgetTypeVersion struct {
	number int
}

var _ fmt.Stringer = WidgetTypeVersion{}

var (
	WidgetTypeVersionInitial   = WidgetTypeVersion{number: 0}
	WidgetTypeVersionCommitted = WidgetTypeVersion{number: 1}
)

// NewWidgetTypeVersion creates a WidgetTypeVersion with the given options.
func NewWidgetTypeVersion(opts ...WidgetTypeVersionOption) (WidgetTypeVersion, error) {
	version := WidgetTypeVersion{number: 0}

	for _, opt := range opts {
		opt(&version)
	}

	if version.number < 0 {
		return WidgetTypeVersion{}, fmt.Errorf("widget type version cannot be negative: %d", version.number)
	}

	return version, nil
}

// WidgetTypeVersionOption - option function for WidgetTypeVersion.
type WidgetTypeVersionOption func(*WidgetTypeVersion)

// WidgetTypeVersionWithNumber sets the version number.
func WidgetTypeVersionWithNumber(number int) WidgetTypeVersionOption {
	return func(v *WidgetTypeVersion) {
		v.number = number
	}
}

// Number returns the underlying integer value.
func (v WidgetTypeVersion) Number() int {
	return v.number
}

// Next returns the incremented version.
func (v WidgetTypeVersion) Next() WidgetTypeVersion {
	return WidgetTypeVersion{number: v.number + 1}
}

// IsCommitted returns true if the version is greater than zero.
func (v WidgetTypeVersion) IsCommitted() bool {
	return v.number > 0
}

// String implements [fmt.Stringer].
func (v WidgetTypeVersion) String() string {
	return fmt.Sprintf("%d", v.number)
}
