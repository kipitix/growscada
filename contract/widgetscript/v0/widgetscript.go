// Package widgetscriptv0 is MAJOR 0 of the WidgetType script contract: the
// `inputs` object a WidgetType's script receives in render(inputs) (ADR
// 0005). It crosses a time boundary: a script is stored in a WidgetType and
// run for years by newer UIs. The script itself is not told the version; it
// is in the schema and in docs/widget_type_contract.md.
package widgetscriptv0

import (
	"encoding/json"

	"github.com/kipitix/growscada/contract"
)

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
	// Value is the port's JS value. The zero Value leaves "value" out, so the
	// script sees undefined.
	Value Value `json:"value,omitzero"`
	// Text is the Tag's value as text, in the canonical form the server keeps
	// it in, for every TagType (in a preview, the typed-in value in that
	// form; as typed for an "any" port). nil (the port has no value) leaves
	// "text" out, so the script tells a missing value from a real 0, false
	// or "".
	Text *string `json:"text,omitempty"`
}

// Value is an input's JS value: a number for Integer (IntegerValue), a
// boolean for Boolean (BooleanValue), a string for String (StringValue). An
// Integer beyond ±(2^53 − 1) is rounded by the script's JSON.parse; Input.Text
// holds it exactly. The zero Value is no value.
type Value struct{ v any }

// IntegerValue is the value of an Integer.
func IntegerValue(n int64) Value { return Value{n} }

// BooleanValue is the value of a Boolean.
func BooleanValue(b bool) Value { return Value{b} }

// StringValue is the value of a String.
func StringValue(s string) Value { return Value{s} }

// IsZero reports whether v is no value; omitzero leaves it out.
func (v Value) IsZero() bool { return v.v == nil }

// MarshalJSON implements json.Marshaler.
func (v Value) MarshalJSON() ([]byte, error) { return json.Marshal(v.v) }

// JSONSchema describes the JS types a value can have for the schema
// generator, so a change of the type a TagType is passed as changes the
// schema.
func (Value) JSONSchema() map[string]any {
	return map[string]any{"type": []any{"integer", "boolean", "string"}}
}
