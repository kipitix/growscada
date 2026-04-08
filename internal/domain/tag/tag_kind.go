package tag

// TagKind - перечисление типов тегов
// Определяет возможные типы значений, которые может содержать тег
//
//go:generate enumer -type=TagKind -trimprefix=TagKind -json
type TagKind int

// Константы для TagKind
// TagKindBoolean - булев тип (true/false)
// TagKindInteger - целочисленный тип
// TagKindFloat - вещественный тип (закомментировано)
// TagKindString - строковый тип (закомментировано)
// TagKindBytes - байтовый массив (закомментировано)
const (
	TagKindBoolean TagKind = iota
	TagKindInteger
	// TagKindFloat
	// TagKindString
	// TagKindBytes
)

// IsValidValue проверяет, соответствует ли значение указанному типу тега
// Возвращает true, если значение может быть присвоено тегу данного типа
func (tt TagKind) IsValidValue(aValue TagValue) bool {
	switch aValue.(type) {
	case bool:
		return tt == TagKindBoolean
	case int:
		return tt == TagKindInteger
	default:
		return false
	}
}
