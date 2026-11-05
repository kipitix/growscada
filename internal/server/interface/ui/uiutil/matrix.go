package uiutil

import (
	"fmt"
	"math"
)

// Matrix is the CSS matrix() of a Widget from its geometry, as the Scene
// aggregate derives the transform matrix:
//
//	T(pos) · T(+ox,+oy) · R(θ) · T(-ox,-oy)
//
// originX and originY are the rotation origin as fractions of the size, as a
// Widget stores it. The result is a, b, c, d, e, f in the order matrix() takes
// them: x' = a·x + c·y + e, y' = b·x + d·y + f.
func Matrix(posX, posY, originX, originY, rotationDegrees float64, width, height int) [6]float64 {
	rad := rotationDegrees * math.Pi / 180
	cosA := math.Cos(rad)
	sinA := math.Sin(rad)
	ox := originX * float64(width)
	oy := originY * float64(height)
	e := posX - cosA*ox + sinA*oy + ox
	f := posY - sinA*ox - cosA*oy + oy
	return [6]float64{cosA, sinA, -sinA, cosA, e, f}
}

// MatrixCSS is Matrix as the value of a CSS transform property.
func MatrixCSS(posX, posY, originX, originY, rotationDegrees float64, width, height int) string {
	m := Matrix(posX, posY, originX, originY, rotationDegrees, width, height)
	return fmt.Sprintf("matrix(%.6f,%.6f,%.6f,%.6f,%.6f,%.6f)", m[0], m[1], m[2], m[3], m[4], m[5])
}
