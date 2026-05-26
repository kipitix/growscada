package widget

import "fmt"

// Origin is the anchor point for rotation and position transformations,
// expressed as relative coordinates within the widget bounds: [0, 1] × [0, 1].
// (0, 0) is the top-left corner; (0.5, 0.5) is the center; (1, 1) is the bottom-right.
// Value Object.
type Origin struct {
	x float64
	y float64
}

var _ fmt.Stringer = Origin{}

// DefaultOrigin returns the center anchor point (0.5, 0.5).
func DefaultOrigin() Origin {
	return Origin{x: 0.5, y: 0.5}
}

// NewOrigin creates an Origin value object. X and Y must be in [0, 1].
func NewOrigin(x, y float64) (Origin, error) {
	if x < 0 || x > 1 {
		return Origin{}, fmt.Errorf("origin x must be in [0, 1], got %.4f", x)
	}
	if y < 0 || y > 1 {
		return Origin{}, fmt.Errorf("origin y must be in [0, 1], got %.4f", y)
	}
	return Origin{x: x, y: y}, nil
}

func (o Origin) X() float64 { return o.x }
func (o Origin) Y() float64 { return o.y }

// String implements [fmt.Stringer].
func (o Origin) String() string {
	return fmt.Sprintf("(%.4f, %.4f)", o.x, o.y)
}
