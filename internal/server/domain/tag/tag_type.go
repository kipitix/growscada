package tag

import (
	"fmt"
)

// TagType - enumeration for tag type.
// The zero value is invalid (see docs/adr/0001-no-unknown-enum-sentinels.md).
// Value Object.
type TagType struct {
	tagType int
}

// Interfaces for TagType
var _ fmt.Stringer = TagType{}

// Constants for each tag type.
// TagTypeString - string tag type.
// TagTypeBoolean - boolean tag type.
// TagTypeInteger - integer tag type.
var (
	TagTypeString  = TagType{tagType: 1}
	TagTypeBoolean = TagType{tagType: 2}
	TagTypeInteger = TagType{tagType: 3}
	// TODO: add remaining types
	// TagTypeFloat
	// TagTypeBytes
)

// NewTagType parses a string into the enum.
// Returns an error for any string that is not a known tag type.
func NewTagType(s string) (TagType, error) {
	switch s {
	case "string":
		return TagTypeString, nil
	case "boolean":
		return TagTypeBoolean, nil
	case "integer":
		return TagTypeInteger, nil
	default:
		return TagType{}, fmt.Errorf("unknown tag type: %q", s)
	}
}

// IsValid reports whether the tag type is one of the known values (not the zero value).
func (e TagType) IsValid() bool {
	return e != TagType{}
}

// String returns the string representation of the tag type
func (e TagType) String() string {
	switch e {
	case TagTypeString:
		return "string"
	case TagTypeBoolean:
		return "boolean"
	case TagTypeInteger:
		return "integer"
	default:
		return "invalid"
	}
}

// NewTagValue creates a new tag value.
// Factory method.
func (t TagType) NewTagValue(aValue any) (TagValue, error) {
	switch t {
	case TagTypeString:
		return NewTagValueString(aValue)
	case TagTypeBoolean:
		return NewTagValueBoolean(aValue)
	case TagTypeInteger:
		return NewTagValueInteger(aValue)
	default:
		return nil, fmt.Errorf("unknown tag type: %v", t)
	}
}
