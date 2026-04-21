package tag

import "fmt"

// TagValue represents the value of a tag.
// Value Object
type TagValue interface {
	Value() any
	String() string
}

// NewTagValue creates a tag value
func NewTagValue(aValue any, aKind TagKind) (TagValue, error) {
	switch aKind {
	case TagKindString:
		return NewTagValueString(aValue)
	case TagKindBoolean:
		return NewTagValueBoolean(aValue)
	case TagKindInteger:
		return NewTagValueInteger(aValue)
	default:
		return nil, fmt.Errorf("unknown tag kind: %v", aKind)
	}
}
