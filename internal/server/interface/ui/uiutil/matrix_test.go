package uiutil

import "testing"

// TestMatrixCSS pins the transform of a Widget: a quarter turn, around the
// top-left corner and around the centre, since both views of the UI now share
// this one formula.
func TestMatrixCSS(t *testing.T) {
	cases := []struct {
		name                         string
		posX, posY, originX, originY float64
		rotationDegrees              float64
		width, height                int
		want                         string
	}{
		{
			name:            "quarter turn around the top-left corner",
			rotationDegrees: 90, width: 100, height: 50,
			want: "matrix(0.000000,1.000000,-1.000000,0.000000,0.000000,0.000000)",
		},
		{
			name: "quarter turn around the centre, translated",
			posX: 10, posY: 20, originX: 0.5, originY: 0.5,
			rotationDegrees: 90, width: 100, height: 50,
			want: "matrix(0.000000,1.000000,-1.000000,0.000000,85.000000,-5.000000)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MatrixCSS(c.posX, c.posY, c.originX, c.originY, c.rotationDegrees, c.width, c.height); got != c.want {
				t.Errorf("MatrixCSS() = %s, want %s", got, c.want)
			}
		})
	}
}
