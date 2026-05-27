package widget

import "fmt"

// Size - dimensions of a widget in pixels on the scene canvas.
// Value Object.
type Size struct {
	width  int
	height int
}

var _ fmt.Stringer = Size{}

// NewSize creates a Size value object. Width and height must be positive.
func NewSize(width, height int) (Size, error) {
	if width <= 0 {
		return Size{}, fmt.Errorf("widget width must be positive, got %d", width)
	}
	if height <= 0 {
		return Size{}, fmt.Errorf("widget height must be positive, got %d", height)
	}
	return Size{width: width, height: height}, nil
}

// DefaultSize returns the default 100×100 size.
func DefaultSize() Size {
	return Size{width: 100, height: 100}
}

func (s Size) Width() int  { return s.width }
func (s Size) Height() int { return s.height }

// String implements [fmt.Stringer].
func (s Size) String() string {
	return fmt.Sprintf("%dx%d", s.width, s.height)
}
