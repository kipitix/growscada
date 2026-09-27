package widget

import "fmt"

// HtmlTemplate - HTML template used to render the widget element.
// Value Object.
type HtmlTemplate struct {
	template string
}

var _ fmt.Stringer = HtmlTemplate{}

// NewHtmlTemplate creates a HtmlTemplate from a string.
func NewHtmlTemplate(aTemplate string) (HtmlTemplate, error) {
	// TODO: add restrictions and validation
	return HtmlTemplate{template: aTemplate}, nil
}

// String implements [fmt.Stringer].
func (h HtmlTemplate) String() string {
	return h.template
}
