package tag

// TagType is an enum for tag type
// TagType is value object for tag type
//
//go:generate enumer -type=TagType -trimprefix=TagType -json
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

// IsValidValue checks if the given value is valid for the tag type
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
