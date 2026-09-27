package tag

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrInvalidTagNameMatcher is returned when a name pattern or regular
// expression cannot be compiled.
var ErrInvalidTagNameMatcher = errors.New("invalid tag name matcher")

// TagNameMatcher - selects tags by name: a wildcard pattern or a regular expression.
// Value Object.
type TagNameMatcher struct {
	re *regexp.Regexp
}

// NewTagNamePattern creates a matcher from a wildcard pattern that must match
// the whole name: "*" matches any sequence of characters (including none),
// "?" matches exactly one character, everything else matches itself.
func NewTagNamePattern(pattern string) (TagNameMatcher, error) {
	var b strings.Builder
	b.WriteString(`(?s)^`)
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(`.*`)
		case '?':
			b.WriteString(`.`)
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString(`$`)
	re, err := regexp.Compile(b.String())
	if err != nil {
		return TagNameMatcher{}, fmt.Errorf("%w: pattern %q: %v", ErrInvalidTagNameMatcher, pattern, err)
	}
	return TagNameMatcher{re: re}, nil
}

// NewTagNameRegex creates a matcher from a regular expression in Go (RE2)
// syntax. It matches anywhere in the name; use ^ and $ to anchor it.
func NewTagNameRegex(expr string) (TagNameMatcher, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return TagNameMatcher{}, fmt.Errorf("%w: regex %q: %v", ErrInvalidTagNameMatcher, expr, err)
	}
	return TagNameMatcher{re: re}, nil
}

// Matches reports whether the name is selected by the matcher.
func (m TagNameMatcher) Matches(aName TagName) bool {
	return m.re != nil && m.re.MatchString(aName.String())
}

// String returns the underlying regular expression.
func (m TagNameMatcher) String() string {
	if m.re == nil {
		return ""
	}
	return m.re.String()
}
