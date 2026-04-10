package tag

import (
	"fmt"
)

// TagQuality - перечисление для качества тега
// Value Object
type TagQuality int

// Значения перечисления качества тега
// TagQualityBad - плохое качество (недоступен, ошибка)
// TagQualityUncertain - неопределенное качество
// TagQualityGood - хорошее качество (нормальное значение)
// TagQualitySimulated - симулированное качество (значение задано вручную)
const (
	TagQualityBad TagQuality = iota
	TagQualityUncertain
	TagQualityGood
	TagQualitySimulated
)

// NewTagQuality парсит строку в enum
func NewTagQuality(s string) (TagQuality, error) {
	switch s {
	case "Bad":
		return TagQualityBad, nil
	case "Uncertain":
		return TagQualityUncertain, nil
	case "Good":
		return TagQualityGood, nil
	case "Simulated":
		return TagQualitySimulated, nil
	default:
		return -1, fmt.Errorf("unknown tag quality enum: %s", s)
	}
}

// String возвращает строковое представление enum
func (e TagQuality) String() string {
	switch e {
	case TagQualityBad:
		return "Bad"
	case TagQualityUncertain:
		return "Uncertain"
	case TagQualityGood:
		return "Good"
	case TagQualitySimulated:
		return "Simulated"
	default:
		return "Unknown"
	}
}

// IsValid проверяет, является ли значение валидным
func (e TagQuality) IsValid() bool {
	return e >= TagQualityBad && e <= TagQualitySimulated
}
