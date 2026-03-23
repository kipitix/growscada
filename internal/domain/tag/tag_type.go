package tag

import (
	"fmt"
	"strings"
)

// TagType is an enum for tag type
// TagType is value object for tag type
type TagType int

// const for TagType
const (
	TagTypeUnknown TagType = iota
	TagTypeBoolean
	TagTypeInteger
	// TagTypeFloat
	// TagTypeString
	// TagTypeBytes
)

// String returns the string representation of the TagType enum
func (tt TagType) String() string {
	switch tt {
	case TagTypeBoolean:
		return "Boolean"
	case TagTypeInteger:
		return "Integer"
	default:
		panic(fmt.Errorf("unknown tag type enum: %d", tt))
	}
}

// TagTypeFromString creates a new TagType value object from a string
func TagTypeFromString(aString string) (TagType, error) {
	switch strings.ToLower(aString) {
	case "boolean":
		return TagTypeBoolean, nil
	case "integer":
		return TagTypeInteger, nil
	default:
		return TagTypeUnknown, fmt.Errorf("invalid tag type: %s", aString)
	}
}

func (tt TagType) IsValidValue(aValue TagValue) bool {
	switch aValue.(type) {
	case bool:
		return tt == TagTypeBoolean
	case int:
		return tt == TagTypeInteger
	default:
		return false
	}
}
