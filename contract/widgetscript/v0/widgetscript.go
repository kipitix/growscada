// Package widgetscriptv0 is MAJOR 0 of the WidgetType script contract: the
// `inputs` object a WidgetType's script receives in render(inputs) (ADR
// 0005). It crosses a time boundary: a script is stored in a WidgetType and
// run for years by newer UIs. The script itself is not told the version; it
// is in the schema and in docs/widget_type_contract.md.
package widgetscriptv0

import "github.com/kipitix/growscada/contract"

// SchemaVersion is the version of the WidgetType script contract this
// package describes.
// A function rather than a variable, so no importer can change it.
func SchemaVersion() contract.SchemaVersion { return contract.SchemaVersion{Major: 0, Minor: 1} }

// Inputs is the `inputs` object passed to render(inputs), keyed by InputPort
// name. Every InputPort of the WidgetType has a key.
type Inputs map[string]Input

// Input is one InputPort's entry: `inputs.<port>`. Fields are only ever added
// to it, never changed.
type Input struct {
	// Value is the port's JS value: a number (int64) for Integer, a boolean
	// for Boolean, a string for String. nil leaves "value" out, so the script
	// sees undefined. An Integer beyond ±(2^53 − 1) is rounded by the
	// script's JSON.parse; Text holds it exactly.
	Value any `json:"value,omitempty"`
	// Text is the Tag's value as text, in the canonical form the server keeps
	// it in, for every TagType. nil (the port has no value) leaves "text"
	// out, so the script tells a missing value from a real 0, false or "".
	Text *string `json:"text,omitempty"`
}
