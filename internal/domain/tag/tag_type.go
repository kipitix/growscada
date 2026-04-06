package tag

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

// Проверка значения на соответствие типу тега
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
