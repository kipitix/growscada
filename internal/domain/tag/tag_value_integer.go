package tag

import (
	"fmt"
	"strconv"
)

type TagValueInteger int

// Interfaces for TagValueInteger
var _ TagValue = TagValueInteger(0)

func NewTagValueInteger(aValue any) (TagValueInteger, error) {
	switch srcValue := aValue.(type) {
	case string:
		intVal, err := strconv.Atoi(srcValue)
		if err != nil {
			return 0, fmt.Errorf("cannot parse string to int: %w", err)
		}
		return TagValueInteger(intVal), nil
	case bool:
		if srcValue {
			return 1, nil
		} else {
			return 0, nil
		}
	case int:
		return TagValueInteger(srcValue), nil
	}
	return 0, fmt.Errorf("unknown tag value type: %T", aValue)
}

func (t TagValueInteger) Value() any {
	return int(t)
}

func (t TagValueInteger) String() string {
	return fmt.Sprintf("%d", t)
}
