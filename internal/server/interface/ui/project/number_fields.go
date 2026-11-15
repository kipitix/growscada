package project

import (
	"fmt"
	"math"
	"strconv"
)

// numberFields parses the numeric fields of a properties editor. A field that
// is not a number cannot be put into a request, so the first one is kept in
// err and nothing is sent; a number the server does not accept (a zero size,
// an origin outside 0–1) is sent as is and rejected by the server, so the
// editor never silently saves something other than what was typed.
type numberFields struct {
	err error
}

func (n *numberFields) int(name, s string) int {
	v, err := strconv.Atoi(s)
	if err != nil && n.err == nil {
		n.err = fmt.Errorf("%s: %q is not a whole number", name, s)
	}
	return v
}

func (n *numberFields) float(name, s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	// NaN and ±Inf parse but have no JSON form, so they cannot be sent either.
	if (err != nil || math.IsNaN(v) || math.IsInf(v, 0)) && n.err == nil {
		n.err = fmt.Errorf("%s: %q is not a number", name, s)
	}
	return v
}

// geometry is float for a field of an object the editor shows rounded: when
// the text still is the shown value, the object's own value is returned
// exactly, so saving another field of the same panel does not round this one;
// a text the user changed is parsed.
func (n *numberFields) geometry(name, text, shown string, exact float64) float64 {
	if text == shown {
		return exact
	}
	return n.float(name, text)
}
