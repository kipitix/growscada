package widget

import "fmt"

// Script - rendering logic for the widget type.
// Value Object.
type Script struct {
	script string
}

var _ fmt.Stringer = Script{}

// NewScript creates a Script from a string.
func NewScript(aScript string) (Script, error) {
	// TODO: add restrictions and validation
	return Script{script: aScript}, nil
}

// String implements [fmt.Stringer].
func (s Script) String() string {
	return s.script
}
