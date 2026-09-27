package widget

import (
	"fmt"
	"math"
)

// TransformationMatrix is the 2D affine transformation matrix for a widget,
// expressed in CSS matrix(a, b, c, d, e, f) form.
//
// The columns map to:
//
//	[ a  c  e ]   [ cos(θ)  -sin(θ)  tx ]
//	[ b  d  f ] = [ sin(θ)   cos(θ)  ty ]
//	[ 0  0  1 ]   [   0        0      1  ]
//
// tx and ty incorporate the widget's position and the origin anchor, so that
// rotation happens around the anchor point rather than the top-left corner.
//
// Value Object.
type TransformationMatrix struct {
	a, b, c, d, e, f float64
}

var _ fmt.Stringer = TransformationMatrix{}

// NewTransformationMatrix computes the 2D affine matrix from the widget's
// position, origin anchor, rotation angle and size.
//
// Transformation sequence (right-to-left):
//
//	T(pos)  ·  T(+ox, +oy)  ·  R(θ)  ·  T(-ox, -oy)
//
// where ox = origin.X * size.Width, oy = origin.Y * size.Height.
func NewTransformationMatrix(pos Position, origin Origin, rot Rotation, size Size) TransformationMatrix {
	rad := rot.Degrees() * math.Pi / 180
	cos := math.Cos(rad)
	sin := math.Sin(rad)

	// Size.Width/Height are integer pixels; explicit float64 cast is required for
	// sub-pixel-accurate matrix arithmetic. If sub-pixel widget dimensions are ever
	// needed, change Size fields to float64 and remove the casts.
	ox := origin.X() * float64(size.Width())
	oy := origin.Y() * float64(size.Height())

	return TransformationMatrix{
		a: cos,
		b: sin,
		c: -sin,
		d: cos,
		e: pos.X() - cos*ox + sin*oy + ox,
		f: pos.Y() - sin*ox - cos*oy + oy,
	}
}

func (m TransformationMatrix) A() float64 { return m.a }
func (m TransformationMatrix) B() float64 { return m.b }
func (m TransformationMatrix) C() float64 { return m.c }
func (m TransformationMatrix) D() float64 { return m.d }
func (m TransformationMatrix) E() float64 { return m.e }
func (m TransformationMatrix) F() float64 { return m.f }

// CSS returns the CSS matrix() string ready for use in inline styles:
//
//	style="transform: matrix(a,b,c,d,e,f)"
func (m TransformationMatrix) CSS() string {
	return fmt.Sprintf("matrix(%.6f,%.6f,%.6f,%.6f,%.6f,%.6f)",
		m.a, m.b, m.c, m.d, m.e, m.f)
}

// String implements [fmt.Stringer].
func (m TransformationMatrix) String() string { return m.CSS() }
