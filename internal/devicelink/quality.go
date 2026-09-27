package devicelink

import (
	"encoding"
	"fmt"
)

// Quality is how trustworthy a value a Device reports is, as the server's REST
// API names it. The zero value is invalid (ADR 0001). It reads and writes
// itself as text, so it can be used directly in YAML and JSON.
type Quality struct {
	name string
}

var (
	_ fmt.Stringer             = Quality{}
	_ encoding.TextMarshaler   = Quality{}
	_ encoding.TextUnmarshaler = (*Quality)(nil)
)

// Qualities. Must not be reassigned.
var (
	QualityBad       = Quality{name: "bad"}
	QualityUncertain = Quality{name: "uncertain"}
	QualityGood      = Quality{name: "good"}
)

// ParseQuality parses the REST API's name of a quality.
func ParseQuality(s string) (Quality, error) {
	for _, q := range []Quality{QualityBad, QualityUncertain, QualityGood} {
		if q.name == s {
			return q, nil
		}
	}
	return Quality{}, fmt.Errorf("unknown quality %q (want good, bad or uncertain)", s)
}

// IsValid reports whether the quality is one of the known values (not the zero value).
func (q Quality) IsValid() bool {
	return q != Quality{}
}

// String returns the REST API's name of the quality.
func (q Quality) String() string {
	if !q.IsValid() {
		return "invalid"
	}
	return q.name
}

// MarshalText implements encoding.TextMarshaler; the zero value is an error.
func (q Quality) MarshalText() ([]byte, error) {
	if !q.IsValid() {
		return nil, fmt.Errorf("invalid quality")
	}
	return []byte(q.name), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (q *Quality) UnmarshalText(text []byte) error {
	parsed, err := ParseQuality(string(text))
	if err != nil {
		return err
	}
	*q = parsed
	return nil
}
