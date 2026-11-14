package label

import (
	"fmt"
	"regexp"
	"strings"
)

// colorPattern is the form of a color: #RRGGBB, six hexadecimal digits after
// the leading '#'.
var colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// Color - the optional color of a Label (task 85_2; ADR 0025). The zero Color
// is empty: the theme's neutral color. A chip always shows the name, so the
// color is never the only difference between labels.
// Value Object.
type Color struct {
	hex string
}

var _ fmt.Stringer = Color{}

// NewColor creates a Color from a string: empty is a valid Color (no color),
// everything else must be #RRGGBB. The stored form is lower case.
func NewColor(aColor string) (Color, error) {
	trimmed := strings.TrimSpace(aColor)
	color := Color{hex: strings.ToLower(trimmed)}
	if err := color.validate(); err != nil {
		return Color{}, err
	}
	return color, nil
}

// validate reports whether the color holds a value NewColor accepts.
func (c Color) validate() error {
	if c.hex == "" || colorPattern.MatchString(c.hex) {
		return nil
	}
	return fmt.Errorf("%w: %q is not #RRGGBB", ErrInvalidLabelColor, c.hex)
}

// FromStorage rebuilds a Color as stored, skipping the format check: the value
// was validated when it was written. For repositories.
func FromStorage(aColor string) Color {
	return Color{hex: strings.ToLower(strings.TrimSpace(aColor))}
}

// IsEmpty reports whether the label has no color.
func (c Color) IsEmpty() bool { return c.hex == "" }

// String implements [fmt.Stringer]: the #RRGGBB value, or "" for no color.
func (c Color) String() string { return c.hex }
