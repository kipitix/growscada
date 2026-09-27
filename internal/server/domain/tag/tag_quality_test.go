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

func TestNewTagQuality_UnrecognisedValue_ReturnsErrorAndInvalid(t *testing.T) {
	for _, input := range []string{"excellent", "simulated", "unknown", ""} {
		t.Run(input, func(t *testing.T) {
			q, err := NewTagQuality(input)
			if err == nil {
				t.Errorf("expected error for %q, got nil", input)
			}
			if q.IsValid() {
				t.Errorf("expected invalid zero value for %q, got %v", input, q)
			}
		})
	}
}

func TestTagQuality_IsValid(t *testing.T) {
	for _, q := range []TagQuality{TagQualityBad, TagQualityUncertain, TagQualityGood} {
		if !q.IsValid() {
			t.Errorf("expected %v to be valid", q)
		}
	}
	if (TagQuality{}).IsValid() {
		t.Error("expected zero value to be invalid")
	}
}

func TestTagQuality_String(t *testing.T) {
	cases := []struct {
		name     string
		quality  TagQuality
		expected string
	}{
		{"zero", TagQuality{}, "invalid"},
		{"bad", TagQualityBad, "bad"},
		{"uncertain", TagQualityUncertain, "uncertain"},
		{"good", TagQualityGood, "good"},
		{"default", TagQuality{quality: 99}, "invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.quality.String(); got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
