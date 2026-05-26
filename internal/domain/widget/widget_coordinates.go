package widget

// NewCoordinates creates a Position from a legacy float64 z argument.
//
// Deprecated: use [NewPosition] instead.
func NewCoordinates(x, y, z float64) Position {
	return NewPosition(x, y, int(z))
}
