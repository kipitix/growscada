package tag

import "fmt"

type TagValueBoolean bool

var _ TagValue = TagValueBoolean(false)

func NewTagValueBoolean(aValue any) (TagValueBoolean, error) {
	switch srcValue := aValue.(type) {
	case string:
		switch srcValue {
		case "true":
			return TagValueBoolean(true), nil
		case "false":
			return TagValueBoolean(false), nil
		default:
			return TagValueBoolean(false), fmt.Errorf("unknown tag value: %v", aValue)
		}
	case bool:
		return TagValueBoolean(srcValue), nil
	case int:
		return TagValueBoolean(srcValue != 0), nil
	}
	return false, fmt.Errorf("unknown tag value type %T", aValue)
}

func (t TagValueBoolean) Value() any {
	return bool(t)
}

func (t TagValueBoolean) String() string {
	if t {
		return "true"
	} else {
		return "false"
	}
}
