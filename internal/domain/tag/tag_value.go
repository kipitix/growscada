package tag

import "fmt"

// TagValue представляет значение тега
// Value Object
type TagValue interface {
	Value() any
	String() string
}

// NewTagValue создает значение тега
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
