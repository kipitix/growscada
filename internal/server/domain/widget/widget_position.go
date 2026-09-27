package widget

import "fmt"

// Position holds the widget's location on the scene canvas.
// x and y are the canvas coordinates in pixels; z is the layer order (z-index).
// Value Object.
type Position struct {
	x float64
	y float64
	z int
}

var _ fmt.Stringer = Position{}

// NewPosition creates a Position value object.
func NewPosition(x, y float64, z int) Position {
	return Position{x: x, y: y, z: z}
}

func (p Position) X() float64 { return p.x }
func (p Position) Y() float64 { return p.y }
func (p Position) Z() int     { return p.z }

// String implements [fmt.Stringer].
func (p Position) String() string {
	return fmt.Sprintf("(%.2f, %.2f, z:%d)", p.x, p.y, p.z)
}
