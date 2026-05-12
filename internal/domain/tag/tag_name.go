package tag

import "fmt"

// TagName - name of the tag.
// Value Object.
type TagName struct {
	name string
}

// Interfaces for TagName.
var _ fmt.Stringer = TagName{}

// Factory method.
func NewTagName(aName string) (TagName, error) {
	// TODO: add restrictions and validation
	return TagName{name: aName}, nil
}

// String method.
func (tn TagName) String() string {
	return tn.name
}
