package devicelink

import "fmt"

// TagType is a Tag's data type, as the server's REST API names it.
// The zero value is invalid (ADR 0001).
type TagType struct {
	name string
}

var _ fmt.Stringer = TagType{}

// Tag types. Must not be reassigned.
var (
	TagTypeString  = TagType{name: "string"}
	TagTypeBoolean = TagType{name: "boolean"}
	TagTypeInteger = TagType{name: "integer"}
)

// ParseTagType parses the REST API's name of a tag type.
func ParseTagType(s string) (TagType, error) {
	for _, t := range []TagType{TagTypeString, TagTypeBoolean, TagTypeInteger} {
		if t.name == s {
			return t, nil
		}
	}
	return TagType{}, fmt.Errorf("unknown tag type %q (want string, boolean or integer)", s)
}

// IsValid reports whether the type is one of the known values (not the zero value).
func (t TagType) IsValid() bool {
	return t != TagType{}
}

// String returns the REST API's name of the type.
func (t TagType) String() string {
	if !t.IsValid() {
		return "invalid"
	}
	return t.name
}
