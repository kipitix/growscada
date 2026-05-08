package tag

import "fmt"

type TagName string

// Interfaces for TagName
var _ fmt.Stringer = TagName("")

// Factory method
func NewTagName(name string) (TagName, error) {
	return TagName(name), nil
}

// String method
func (tn TagName) String() string {
	return string(tn)
}
