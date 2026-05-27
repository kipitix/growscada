package project

import (
	"fmt"
	"math"
)

// ── Matrix helper ─────────────────────────────────────────────────────────────

// computeMatrixCSS calculates the CSS matrix() string from widget geometric
// properties. Mirrors the domain TransformationMatrix formula:
//
//	T(pos) · T(+ox,+oy) · R(θ) · T(-ox,-oy)
func computeMatrixCSS(posX, posY, originX, originY, rotDeg float64, width, height int) string {
	rad := rotDeg * math.Pi / 180
	cosA := math.Cos(rad)
	sinA := math.Sin(rad)
	ox := originX * float64(width)
	oy := originY * float64(height)
	e := posX - cosA*ox + sinA*oy + ox
	f := posY - sinA*ox - cosA*oy + oy
	return fmt.Sprintf("matrix(%.6f,%.6f,%.6f,%.6f,%.6f,%.6f)",
		cosA, sinA, -sinA, cosA, e, f)
}

func widgetMatrixCSS(w widgetItem) string {
	return computeMatrixCSS(
		w.Position.X, w.Position.Y,
		w.Origin.X, w.Origin.Y,
		w.Rotation.Degrees,
		w.Size.Width, w.Size.Height,
	)
}
