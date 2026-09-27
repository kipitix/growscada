package tag

import (
	"fmt"
	"strconv"
)

// TagValueInteger stores a 64-bit signed integer tag value.
// Using int64 guarantees consistent range across all platforms and matches
// the full range of industrial protocol values (Modbus, OPC-UA, MQTT).
type TagValueInteger int64

// Interfaces for TagValueInteger
var _ TagValue = TagValueInteger(0)

func NewTagValueInteger(aValue any) (TagValueInteger, error) {
	switch srcValue := aValue.(type) {
	case string:
		intVal, err := strconv.ParseInt(srcValue, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot parse string to int64: %w", err)
		}
		return TagValueInteger(intVal), nil
	case bool:
		if srcValue {
			return 1, nil
		}
		return 0, nil
	case int:
		return TagValueInteger(int64(srcValue)), nil
	case int32:
		return TagValueInteger(int64(srcValue)), nil
	case int64:
		return TagValueInteger(srcValue), nil
	}
	return 0, fmt.Errorf("unknown tag value type: %T", aValue)
}

// Value returns the underlying int64. Callers must assert to int64, not int.
func (t TagValueInteger) Value() any {
	return int64(t)
}

func (t TagValueInteger) String() string {
	return fmt.Sprintf("%d", t)
}
