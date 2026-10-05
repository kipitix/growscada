package scene

import "fmt"

// WidgetSize - dimensions of a widget in pixels on the scene canvas.
// Value Object.
type WidgetSize struct {
	width  int
	height int
}

var _ fmt.Stringer = WidgetSize{}

// NewWidgetSize creates a WidgetSize value object. Width and height must be positive.
func NewWidgetSize(width, height int) (WidgetSize, error) {
	if width <= 0 {
		return WidgetSize{}, fmt.Errorf("widget width must be positive, got %d", width)
	}
	if height <= 0 {
		return WidgetSize{}, fmt.Errorf("widget height must be positive, got %d", height)
	}
	return WidgetSize{width: width, height: height}, nil
}

// DefaultWidgetSize returns the default 100×100 size.
func DefaultWidgetSize() WidgetSize {
	return WidgetSize{width: 100, height: 100}
}

func (s WidgetSize) Width() int  { return s.width }
func (s WidgetSize) Height() int { return s.height }

// String implements [fmt.Stringer].
func (s WidgetSize) String() string {
	return fmt.Sprintf("%dx%d", s.width, s.height)
}
