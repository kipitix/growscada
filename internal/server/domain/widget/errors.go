package widget

import "errors"

var (
	// ErrWidgetNotFound is returned when a widget cannot be located within its scene.
	ErrWidgetNotFound = errors.New("widget not found")
	// ErrWidgetInvalidInput is returned when required fields are missing or invalid.
	ErrWidgetInvalidInput = errors.New("widget invalid input")
)
