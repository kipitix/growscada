package tag

import (
	"fmt"
)

// TagQuality - enumeration for tag quality.
// Describes how trustworthy a value is, never where it came from.
// The zero value is invalid (see docs/adr/0001-no-unknown-enum-sentinels.md).
// Value Object.
type TagQuality struct {
	quality int
}

// Interfaces for TagQuality
var _ fmt.Stringer = TagQuality{}

// Sentinel values for TagQuality. Must not be reassigned.
// TagQualityBad - bad quality (unavailable, error).
// TagQualityUncertain - uncertain quality.
// TagQualityGood - good quality (normal value).
var (
	TagQualityBad       = TagQuality{quality: 1}
	TagQualityUncertain = TagQuality{quality: 2}
	TagQualityGood      = TagQuality{quality: 3}
)

// NewTagQuality parses a string into the enum.
// Returns an error for any string that is not a known quality.
func NewTagQuality(s string) (TagQuality, error) {
	switch s {
	case "bad":
		return TagQualityBad, nil
	case "uncertain":
		return TagQualityUncertain, nil
	case "good":
		return TagQualityGood, nil
	default:
		return TagQuality{}, fmt.Errorf("unknown tag quality enum: %q", s)
	}
}

// IsValid reports whether the quality is one of the known values (not the zero value).
func (e TagQuality) IsValid() bool {
	return e != TagQuality{}
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
	default:
		return "invalid"
	}
}
