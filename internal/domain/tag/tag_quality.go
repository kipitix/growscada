package tag

import (
	"fmt"
)

// TagQuality - enumeration for tag quality.
// Value Object
type TagQuality int

// Interfaces for TagQuality
var _ fmt.Stringer = TagQuality(0)

// Tag quality enumeration values.
// TagQualityBad - bad quality (unavailable, error).
// TagQualityUncertain - uncertain quality.
// TagQualityGood - good quality (normal value).
// TagQualitySimulated - simulated quality (value set manually).
const (
	TagQualityBad TagQuality = iota
	TagQualityUncertain
	TagQualityGood
	TagQualitySimulated
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
		return -1, fmt.Errorf("unknown tag quality enum: %s", s)
	}
}

// String returns the string representation of the enum
func (e TagQuality) String() string {
	switch e {
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

// IsValid checks whether the value is valid
func (e TagQuality) IsValid() bool {
	return e >= TagQualityBad && e <= TagQualitySimulated
}
