package widget

import (
	"fmt"
	"math"
)

// Rotation represents the rotation angle of a widget in degrees,
// normalized to [0, 360).
// Value Object.
type Rotation struct {
	degrees float64
}

var _ fmt.Stringer = Rotation{}

// DefaultRotation returns a zero rotation.
func DefaultRotation() Rotation {
	return Rotation{degrees: 0}
}

// NewRotation creates a Rotation value object.
// The angle is normalized to [0, 360) regardless of the input value.
func NewRotation(degrees float64) Rotation {
	normalized := math.Mod(degrees, 360)
	if normalized < 0 {
		normalized += 360
	}
	return Rotation{degrees: normalized}
}

func (r Rotation) Degrees() float64 { return r.degrees }

// String implements [fmt.Stringer].
func (r Rotation) String() string {
	return fmt.Sprintf("%.4f°", r.degrees)
}
