package tag

// TagQuality is an enum for tag quality
// TagQuality is value object for tag quality
//
//go:generate enumer -type=TagQuality -trimprefix=TagQuality -json
type TagQuality int

// Tag quality enum values
const (
	TagQualityUnknown TagQuality = iota
	TagQualityBad
	TagQualityUncertain
	TagQualityGood
	TagQualitySimulated
)
