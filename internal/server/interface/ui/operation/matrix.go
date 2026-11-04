package operation

import (
	"fmt"
	"math"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// revisionMatrixCSS derives the CSS matrix() of a Widget of the Revision from
// its geometry, as the Scene aggregate does:
//
//	T(pos) · T(+ox,+oy) · R(θ) · T(-ox,-oy)
func revisionMatrixCSS(widget projectv0.Widget) string {
	rad := widget.Rotation.Degrees * math.Pi / 180
	cosA := math.Cos(rad)
	sinA := math.Sin(rad)
	ox := widget.Origin.X * float64(widget.Size.Width)
	oy := widget.Origin.Y * float64(widget.Size.Height)
	e := widget.Position.X - cosA*ox + sinA*oy + ox
	f := widget.Position.Y - sinA*ox - cosA*oy + oy
	return fmt.Sprintf("matrix(%.6f,%.6f,%.6f,%.6f,%.6f,%.6f)", cosA, sinA, -sinA, cosA, e, f)
}
