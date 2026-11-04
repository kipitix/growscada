package uiutil

import (
	"fmt"
	"math"
)

// MatrixCSS is the CSS matrix() of a Widget from its geometry, as the Scene
// aggregate derives the transform matrix:
//
//	T(pos) · T(+ox,+oy) · R(θ) · T(-ox,-oy)
//
// originX and originY are the rotation origin as fractions of the size, as a
// Widget stores it.
func MatrixCSS(posX, posY, originX, originY, rotationDegrees float64, width, height int) string {
	rad := rotationDegrees * math.Pi / 180
	cosA := math.Cos(rad)
	sinA := math.Sin(rad)
	ox := originX * float64(width)
	oy := originY * float64(height)
	e := posX - cosA*ox + sinA*oy + ox
	f := posY - sinA*ox - cosA*oy + oy
	return fmt.Sprintf("matrix(%.6f,%.6f,%.6f,%.6f,%.6f,%.6f)", cosA, sinA, -sinA, cosA, e, f)
}
