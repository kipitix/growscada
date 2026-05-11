package indicator_type

import "fmt"

// SvgTemplate - SVG template used to render the indicator element.
type SvgTemplate string

// Interfaces for SvgTemplate.
var _ fmt.Stringer = SvgTemplate("")

// NewSvgTemplate creates a SvgTemplate from a string.
func NewSvgTemplate(s string) (SvgTemplate, error) {
	return SvgTemplate(s), nil
}

// String implements [fmt.Stringer].
func (s SvgTemplate) String() string {
	return string(s)
}
