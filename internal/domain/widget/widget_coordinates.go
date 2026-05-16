package widget

import "fmt"

// Coordinates - position of a widget on the dashboard canvas.
// Value Object.
type Coordinates struct {
	x float64
	y float64
	z float64
}

var _ fmt.Stringer = Coordinates{}

// NewCoordinates creates a Coordinates value object.
func NewCoordinates(x, y, z float64) Coordinates {
	return Coordinates{x: x, y: y, z: z}
}

func (c Coordinates) X() float64 { return c.x }
func (c Coordinates) Y() float64 { return c.y }
func (c Coordinates) Z() float64 { return c.z }

// String implements [fmt.Stringer].
func (c Coordinates) String() string {
	return fmt.Sprintf("(%.2f, %.2f, %.2f)", c.x, c.y, c.z)
}
