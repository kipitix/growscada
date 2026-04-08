package tag

// TagQuality - перечисление для качества тега
// Представляет собой value object для оценки достоверности значения тега
//
//go:generate enumer -type=TagQuality -trimprefix=TagQuality -json
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
