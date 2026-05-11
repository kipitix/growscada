package indicator_type

import "fmt"

// Script - rendering logic for the indicator type.
type Script string

// Interfaces for Script.
var _ fmt.Stringer = Script("")

// NewScript creates a Script from a string.
func NewScript(s string) (Script, error) {
	return Script(s), nil
}

// String implements [fmt.Stringer].
func (s Script) String() string {
	return string(s)
}
