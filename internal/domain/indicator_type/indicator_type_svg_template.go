package indicator_type

import "fmt"

// SvgTemplate - SVG template used to render the indicator element.
// Value Object.
type SvgTemplate struct {
	template string
}

// Interfaces for SvgTemplate.
var _ fmt.Stringer = SvgTemplate{}

// NewSvgTemplate creates a SvgTemplate from a string.
func NewSvgTemplate(aTemplate string) (SvgTemplate, error) {
	// TODO: add restrictions and validation
	return SvgTemplate{template: aTemplate}, nil
}

// String method.
func (s SvgTemplate) String() string {
	return s.template
}
