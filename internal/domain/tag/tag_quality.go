package tag

import (
	"fmt"
	"strings"
)

// TagQuality is an enum for tag quality
// TagQuality is value object for tag quality
type TagQuality int

// Tag quality enum values
const (
	TagQualityUnknown TagQuality = iota
	TagQualityBad
	TagQualityUncertain
	TagQualityGood
	TagQualitySimulated
)

// String returns the string representation of the tag quality enum
func (tq TagQuality) String() string {
	switch tq {
	case TagQualityBad:
		return "Bad"
	case TagQualityUncertain:
		return "Uncertain"
	case TagQualityGood:
		return "Good"
	case TagQualitySimulated:
		return "Simulated"
	default:
		panic(fmt.Errorf("unknown tag quality enum: %d", tq))
	}
}

func TagQualityFromString(aString string) (TagQuality, error) {
	switch strings.ToLower(aString) {
	case "bad":
		return TagQualityBad, nil
	case "uncertain":
		return TagQualityUncertain, nil
	case "good":
		return TagQualityGood, nil
	case "simulated":
		return TagQualitySimulated, nil
	default:
		return TagQualityUnknown, fmt.Errorf("invalid tag quality: %s", aString)
	}
}
