package tag

import (
	"errors"
	"testing"
)

func mustName(t *testing.T, s string) TagName {
	t.Helper()
	n, err := NewTagName(s)
	if err != nil {
		t.Fatalf("NewTagName(%q): %v", s, err)
	}
	return n
}

func TestNewTagNamePattern_Matches(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"pump1.speed", "pump1.speed", true},
		{"pump1.speed", "pump1Xspeed", false}, // "." is literal
		{"pump*", "pump1.speed", true},
		{"pump*", "pump", true},
		{"pump*", "xpump1", false}, // matches the whole name
		{"*.speed", "pump1.speed", true},
		{"*.speed", "pump1.speed.raw", false},
		{"pump?.speed", "pump1.speed", true},
		{"pump?.speed", "pump12.speed", false},
		{"pump?.speed", "pump.speed", false},
		{"*", "", true},
		{"?", "ж", true}, // one character, not one byte
		{"a+b(c)[d]", "a+b(c)[d]", true},
		{"a+b", "aab", false}, // regex metacharacters are literal
		{"*line*", "multi\nline", true},
	}
	for _, tt := range tests {
		m, err := NewTagNamePattern(tt.pattern)
		if err != nil {
			t.Fatalf("NewTagNamePattern(%q): %v", tt.pattern, err)
		}
		if got := m.Matches(mustName(t, tt.name)); got != tt.want {
			t.Errorf("pattern %q, name %q: got %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestNewTagNameRegex_Matches(t *testing.T) {
	tests := []struct {
		expr, name string
		want       bool
	}{
		{`speed`, "pump1.speed", true}, // unanchored
		{`^speed`, "pump1.speed", false},
		{`^pump\d+\.speed$`, "pump12.speed", true},
		{`^pump\d+\.speed$`, "pumpX.speed", false},
		{`(?i)^PUMP`, "pump1", true},
	}
	for _, tt := range tests {
		m, err := NewTagNameRegex(tt.expr)
		if err != nil {
			t.Fatalf("NewTagNameRegex(%q): %v", tt.expr, err)
		}
		if got := m.Matches(mustName(t, tt.name)); got != tt.want {
			t.Errorf("regex %q, name %q: got %v, want %v", tt.expr, tt.name, got, tt.want)
		}
	}
}

func TestNewTagNameRegex_Invalid_ReturnsErrInvalidTagNameMatcher(t *testing.T) {
	_, err := NewTagNameRegex(`pump(`)

	if !errors.Is(err, ErrInvalidTagNameMatcher) {
		t.Errorf("expected ErrInvalidTagNameMatcher, got %v", err)
	}
}

func TestTagNameMatcher_ZeroValue_MatchesNothing(t *testing.T) {
	if (TagNameMatcher{}).Matches(mustName(t, "anything")) {
		t.Error("zero TagNameMatcher must match nothing")
	}
}
