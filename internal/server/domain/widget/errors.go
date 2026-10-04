package widget

import "errors"

// ErrWidgetNotFound is returned when a widget cannot be located within its scene.
var ErrWidgetNotFound = errors.New("widget not found")
