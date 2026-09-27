package tag

import "fmt"

type TagValueString string

// Interfaces for TagValueString
var _ TagValue = TagValueString("")

// Factory method
func NewTagValueString(aValue any) (TagValueString, error) {
	switch srcValue := aValue.(type) {
	case string:
		return TagValueString(srcValue), nil
	case bool:
		return TagValueString(fmt.Sprintf("%v", srcValue)), nil
	case int:
		return TagValueString(fmt.Sprintf("%v", srcValue)), nil
	}
	return "", fmt.Errorf("unknown tag value type: %T", aValue)
}

func (t TagValueString) Value() any {
	return string(t)
}

func (t TagValueString) String() string {
	return string(t)
}
