package tag

import (
	"fmt"
)

// TagKind - перечисление для типа тега
// Value Object
type TagKind int

// Константы для каждого типа тега
// TagKindString - строковый тип тега
// TagKindBoolean - логический тип тега
// TagKindInteger - целочисленный тип тега
const (
	TagKindString TagKind = iota
	TagKindBoolean
	TagKindInteger
	// TDOO: Добавить остальные типы
	// TagKindFloat
	// TagKindBytes
)

// NewTagKind парсит строку в enum
func NewTagKind(s string) (TagKind, error) {
	switch s {
	case "string":
		return TagKindString, nil
	case "boolean":
		return TagKindBoolean, nil
	case "integer":
		return TagKindInteger, nil
	default:
		return -1, fmt.Errorf("unknown tag kind: %s", s)
	}
}

// String возвращает строковое представление типа тега
func (e TagKind) String() string {
	switch e {
	case TagKindString:
		return "string"
	case TagKindBoolean:
		return "boolean"
	case TagKindInteger:
		return "integer"
	default:
		return "unknown"
	}
}

// IsValid проверяет, является ли значение допустимым для типа тега
func (t TagKind) IsValid() bool {
	return t >= TagKindString && t <= TagKindInteger
}
