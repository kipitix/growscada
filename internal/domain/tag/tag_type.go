package tag

import (
	"fmt"
)

// TagType - enumeration for tag type.
// Value Object.
type TagType struct {
	tagType int
}

// Interfaces for TagType
var _ fmt.Stringer = TagType{}

// Constants for each tag type.
// TagTypeUnknown - unknown tag type (zero value, uninitialized).
// TagTypeString - string tag type.
// TagTypeBoolean - boolean tag type.
// TagTypeInteger - integer tag type.
var (
	TagTypeUnknown = TagType{tagType: 0}
	TagTypeString  = TagType{tagType: 1}
	TagTypeBoolean = TagType{tagType: 2}
	TagTypeInteger = TagType{tagType: 3}
	// TODO: add remaining types
	// TagTypeFloat
	// TagTypeBytes
)

// NewTagType parses a string into the enum.
// Empty string and "unknown" both map to TagTypeUnknown without error,
// so callers do not need special-case guards for those sentinel values.
func NewTagType(s string) (TagType, error) {
	switch s {
	case "string":
		return TagTypeString, nil
	case "boolean":
		return TagTypeBoolean, nil
	case "integer":
		return TagTypeInteger, nil
	case "unknown", "":
		return TagTypeUnknown, nil
	default:
		return TagTypeUnknown, fmt.Errorf("unknown tag type: %s", s)
	}
}

// String returns the string representation of the tag type
func (e TagType) String() string {
	switch e {
	case TagTypeUnknown:
		return "unknown"
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
