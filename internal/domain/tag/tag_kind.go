package tag

import (
	"fmt"
)

// TagKind - enumeration for tag type.
// Value Object
type TagKind int

// Constants for each tag type.
// TagKindString - string tag type.
// TagKindBoolean - boolean tag type.
// TagKindInteger - integer tag type.
const (
	TagKindString TagKind = iota
	TagKindBoolean
	TagKindInteger
	// TODO: add remaining types
	// TagKindFloat
	// TagKindBytes
)

// NewTagKind parses a string into the enum
func NewTagKind(s string) (TagKind, error) {
	switch s {
	case "string":
		return TagKindString, nil
	case "boolean":
		return TagKindBoolean, nil
	case "integer":
		return TagKindInteger, nil
	default:
		return -1, fmt.Errorf("unknown tag kind: %s", s)
	}
}

// String returns the string representation of the tag type
func (e TagKind) String() string {
	switch e {
	case TagKindString:
		return "string"
	case TagKindBoolean:
		return "boolean"
	case TagKindInteger:
		return "integer"
	default:
		return "unknown"
	}
}

// IsValid checks whether the value is valid for the tag type
func (t TagKind) IsValid() bool {
	return t >= TagKindString && t <= TagKindInteger
}

// NewTagValue creates a new tag value
func (t TagKind) NewTagValue(aValue any) (TagValue, error) {
	switch t {
	case TagKindString:
		return NewTagValueString(aValue)
	case TagKindBoolean:
		return NewTagValueBoolean(aValue)
	case TagKindInteger:
		return NewTagValueInteger(aValue)
	default:
		return nil, fmt.Errorf("unknown tag kind: %v", t)
	}
}
