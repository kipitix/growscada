package tag

import (
	"fmt"
)

// TagQuality - enumeration for tag quality.
// Value Object.
type TagQuality struct {
	quality int
}

// Interfaces for TagQuality
var _ fmt.Stringer = TagQuality{}

// Sentinel values for TagQuality. Must not be reassigned.
// TagQualityUnknown - unknown quality (zero value, uninitialized).
// TagQualityBad - bad quality (unavailable, error).
// TagQualityUncertain - uncertain quality.
// TagQualityGood - good quality (normal value).
// TagQualitySimulated - simulated quality (value set manually).
var (
	TagQualityUnknown   = TagQuality{quality: 0}
	TagQualityBad       = TagQuality{quality: 1}
	TagQualityUncertain = TagQuality{quality: 2}
	TagQualityGood      = TagQuality{quality: 3}
	TagQualitySimulated = TagQuality{quality: 4}
)

// NewTagQuality parses a string into the enum
func NewTagQuality(s string) (TagQuality, error) {
	switch s {
	case "bad":
		return TagQualityBad, nil
	case "uncertain":
		return TagQualityUncertain, nil
	case "good":
		return TagQualityGood, nil
	case "simulated":
		return TagQualitySimulated, nil
	default:
		return TagQualityUnknown, fmt.Errorf("unknown tag quality enum: %s", s)
	}
}

// String returns the string representation of the enum
func (e TagQuality) String() string {
	switch e {
	case TagQualityUnknown:
		return "unknown"
	case TagQualityBad:
		return "bad"
	case TagQualityUncertain:
		return "uncertain"
	case TagQualityGood:
		return "good"
	case TagQualitySimulated:
		return "simulated"
	default:
		return "unknown"
	}
}
