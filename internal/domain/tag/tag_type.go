package tag

import (
	"fmt"
)

// TagType - enumeration for tag type.
// Value Object
type TagType int

// Interfaces for TagType
var _ fmt.Stringer = TagType(0)

// Constants for each tag type.
// TagTypeString - string tag type.
// TagTypeBoolean - boolean tag type.
// TagTypeInteger - integer tag type.
const (
	TagTypeString TagType = iota
	TagTypeBoolean
	TagTypeInteger
	// TODO: add remaining types
	// TagTypeFloat
	// TagTypeBytes
)

// NewTagType parses a string into the enum
func NewTagType(s string) (TagType, error) {
	switch s {
	case "string":
		return TagTypeString, nil
	case "boolean":
		return TagTypeBoolean, nil
	case "integer":
		return TagTypeInteger, nil
	default:
		return -1, fmt.Errorf("unknown tag type: %s", s)
	}
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
		return "unknown"
	}
}

// IsValid checks whether the value is valid for the tag type
func (t TagType) IsValid() bool {
	return t >= TagTypeString && t <= TagTypeInteger
}

// NewTagValue creates a new tag value
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
