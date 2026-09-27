package widget

import (
	"math"
	"strings"
	"testing"
)

const matrixEpsilon = 1e-9

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < matrixEpsilon
}

// TestNewTransformationMatrix_ZeroRotation verifies that at 0° the matrix is a
// pure translation: [1, 0, 0, 1, posX, posY].
func TestNewTransformationMatrix_ZeroRotation(t *testing.T) {
	pos := NewPosition(50, 80, 0)
	origin, _ := NewOrigin(0.5, 0.5)
	rot := NewRotation(0)
	size, _ := NewSize(100, 100)

	m := NewTransformationMatrix(pos, origin, rot, size)

	if !approxEqual(m.A(), 1) {
		t.Errorf("A: expected 1, got %.10f", m.A())
	}
	if !approxEqual(m.B(), 0) {
		t.Errorf("B: expected 0, got %.10f", m.B())
	}
	if !approxEqual(m.C(), 0) {
		t.Errorf("C: expected 0, got %.10f", m.C())
	}
	if !approxEqual(m.D(), 1) {
		t.Errorf("D: expected 1, got %.10f", m.D())
	}
	if !approxEqual(m.E(), 50) {
		t.Errorf("E (tx): expected 50, got %.10f", m.E())
	}
	if !approxEqual(m.F(), 80) {
		t.Errorf("F (ty): expected 80, got %.10f", m.F())
	}
}

// TestNewTransformationMatrix_ZeroRotation_OriginTopLeft checks that with origin
// at (0,0) the translation matches the position directly.
func TestNewTransformationMatrix_ZeroRotation_OriginTopLeft(t *testing.T) {
	pos := NewPosition(30, 40, 0)
	origin, _ := NewOrigin(0, 0)
	rot := NewRotation(0)
	size, _ := NewSize(200, 100)

	m := NewTransformationMatrix(pos, origin, rot, size)

	if !approxEqual(m.E(), 30) {
		t.Errorf("E: expected 30, got %.10f", m.E())
	}
	if !approxEqual(m.F(), 40) {
		t.Errorf("F: expected 40, got %.10f", m.F())
	}
}

// TestNewTransformationMatrix_90Degrees verifies the 90° rotation matrix.
// cos(90°)=0, sin(90°)=1 → a=0, b=1, c=-1, d=0.
func TestNewTransformationMatrix_90Degrees(t *testing.T) {
	pos := NewPosition(0, 0, 0)
	origin, _ := NewOrigin(0, 0) // top-left: ox=0, oy=0
	rot := NewRotation(90)
	size, _ := NewSize(100, 100)

	m := NewTransformationMatrix(pos, origin, rot, size)

	if !approxEqual(m.A(), 0) {
		t.Errorf("A: expected 0, got %.10f", m.A())
	}
	if !approxEqual(m.B(), 1) {
		t.Errorf("B: expected 1, got %.10f", m.B())
	}
	if !approxEqual(m.C(), -1) {
		t.Errorf("C: expected -1, got %.10f", m.C())
	}
	if !approxEqual(m.D(), 0) {
		t.Errorf("D: expected 0, got %.10f", m.D())
	}
	if !approxEqual(m.E(), 0) {
		t.Errorf("E: expected 0, got %.10f", m.E())
	}
	if !approxEqual(m.F(), 0) {
		t.Errorf("F: expected 0, got %.10f", m.F())
	}
}

// TestNewTransformationMatrix_180Degrees verifies the 180° rotation matrix.
// cos(180°)=-1, sin(180°)=0 → identity with negation.
func TestNewTransformationMatrix_180Degrees(t *testing.T) {
	pos := NewPosition(10, 20, 0)
	origin, _ := NewOrigin(0, 0) // top-left anchor: ox=0, oy=0
	rot := NewRotation(180)
	size, _ := NewSize(50, 50)

	m := NewTransformationMatrix(pos, origin, rot, size)

	if !approxEqual(m.A(), -1) {
		t.Errorf("A: expected -1, got %.10f", m.A())
	}
	if !approxEqual(m.D(), -1) {
		t.Errorf("D: expected -1, got %.10f", m.D())
	}
	if !approxEqual(m.B(), 0) {
		t.Errorf("B: expected 0, got %.10f", m.B())
	}
	if !approxEqual(m.C(), 0) {
		t.Errorf("C: expected 0, got %.10f", m.C())
	}
	// tx = posX - cos*ox + sin*oy + ox = 10 - (-1)*0 + 0*0 + 0 = 10
	if !approxEqual(m.E(), 10) {
		t.Errorf("E: expected 10, got %.10f", m.E())
	}
	// ty = posY - sin*ox - cos*oy + oy = 20 - 0*0 - (-1)*0 + 0 = 20
	if !approxEqual(m.F(), 20) {
		t.Errorf("F: expected 20, got %.10f", m.F())
	}
}

// TestNewTransformationMatrix_CenterOriginPreservesPosition checks that with center
// origin and zero rotation, the widget's visual position (top-left of bounding box)
// equals the stored position — the anchor shift cancels out.
func TestNewTransformationMatrix_CenterOriginPreservesPosition(t *testing.T) {
	pos := NewPosition(200, 300, 0)
	origin, _ := NewOrigin(0.5, 0.5)
	rot := NewRotation(0)
	size, _ := NewSize(80, 60)

	m := NewTransformationMatrix(pos, origin, rot, size)

	// With zero rotation the translation column (E, F) must equal pos.
	if !approxEqual(m.E(), 200) {
		t.Errorf("E: expected 200, got %.10f", m.E())
	}
	if !approxEqual(m.F(), 300) {
		t.Errorf("F: expected 300, got %.10f", m.F())
	}
}

func TestTransformationMatrix_CSS(t *testing.T) {
	pos := NewPosition(0, 0, 0)
	origin, _ := NewOrigin(0, 0)
	rot := NewRotation(0)
	size, _ := NewSize(100, 100)

	m := NewTransformationMatrix(pos, origin, rot, size)
	css := m.CSS()

	if !strings.HasPrefix(css, "matrix(") {
		t.Errorf("CSS() should start with 'matrix(', got %q", css)
	}
	if !strings.HasSuffix(css, ")") {
		t.Errorf("CSS() should end with ')', got %q", css)
	}
}

func TestTransformationMatrix_StringMatchesCSS(t *testing.T) {
	pos := NewPosition(10, 20, 0)
	origin, _ := NewOrigin(0.5, 0.5)
	rot := NewRotation(45)
	size, _ := NewSize(100, 100)

	m := NewTransformationMatrix(pos, origin, rot, size)

	if m.String() != m.CSS() {
		t.Errorf("String() != CSS(): %q vs %q", m.String(), m.CSS())
	}
}

// TestNewTransformationMatrix_ViaWidget verifies that Widget.TransformationMatrix()
// delegates correctly to NewTransformationMatrix.
func TestNewTransformationMatrix_ViaWidget(t *testing.T) {
	pos := NewPosition(15, 25, 0)
	origin, _ := NewOrigin(0.5, 0.5)
	rot := NewRotation(0)
	size, _ := NewSize(100, 100)

	direct := NewTransformationMatrix(pos, origin, rot, size)

	w := makeTestWidget(t, pos, size, origin, rot)
	viaWidget := w.TransformationMatrix()

	if direct.CSS() != viaWidget.CSS() {
		t.Errorf("Widget.TransformationMatrix() differs from NewTransformationMatrix(): %q vs %q",
			viaWidget.CSS(), direct.CSS())
	}
}
