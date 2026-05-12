package indicator_type

import "fmt"

// Script - rendering logic for the indicator type.
// Value Object.
type Script struct {
	script string
}

// Interfaces for Script.
var _ fmt.Stringer = Script{}

// NewScript creates a Script from a string.
func NewScript(aScript string) (Script, error) {
	// TODO: add restrictions and validation
	return Script{script: aScript}, nil
}

// String method.
func (s Script) String() string {
	return s.script
}
