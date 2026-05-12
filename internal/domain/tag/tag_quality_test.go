package tag

import "testing"

func TestNewTagQuality_ValidValues(t *testing.T) {
	cases := []struct {
		input    string
		expected TagQuality
	}{
		{"bad", TagQualityBad},
		{"uncertain", TagQualityUncertain},
		{"good", TagQualityGood},
		{"simulated", TagQualitySimulated},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			q, err := NewTagQuality(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if q != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, q)
			}
		})
	}
}

func TestNewTagQuality_UnknownValue(t *testing.T) {
	_, err := NewTagQuality("excellent")
	if err == nil {
		t.Error("expected error for unknown quality, got nil")
	}
}

func TestTagQuality_String(t *testing.T) {
	cases := []struct {
		quality  TagQuality
		expected string
	}{
		{TagQualityUnknown, "unknown"},
		{TagQualityBad, "bad"},
		{TagQualityUncertain, "uncertain"},
		{TagQualityGood, "good"},
		{TagQualitySimulated, "simulated"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			if got := tc.quality.String(); got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
