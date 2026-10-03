package growctl

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

const (
	patternFlag = "pattern"
	regexFlag   = "regex"
)

// NameFilter selects tags by name with a wildcard pattern or a regular
// expression, with the same semantics as the server's GET /tags?name_pattern=
// and ?name_regex=. The zero value selects nothing and IsSet reports false.
type NameFilter struct {
	param   string // REST query parameter: apiv0.TagsQueryNamePattern or TagsQueryNameRegex
	expr    string
	matcher tag.TagNameMatcher
}

// NewPatternFilter creates a filter from a wildcard pattern over the whole
// name: "*" matches any sequence of characters, "?" exactly one.
func NewPatternFilter(pattern string) (NameFilter, error) {
	m, err := tag.NewTagNamePattern(pattern)
	if err != nil {
		return NameFilter{}, err
	}
	return NameFilter{param: apiv0.TagsQueryNamePattern, expr: pattern, matcher: m}, nil
}

// NewRegexFilter creates a filter from a Go (RE2) regular expression matched
// anywhere in the name.
func NewRegexFilter(expr string) (NameFilter, error) {
	m, err := tag.NewTagNameRegex(expr)
	if err != nil {
		return NameFilter{}, err
	}
	return NameFilter{param: apiv0.TagsQueryNameRegex, expr: expr, matcher: m}, nil
}

// IsSet reports whether the filter was given.
func (f NameFilter) IsSet() bool { return f.param != "" }

// Matches reports whether a tag name is selected by the filter.
func (f NameFilter) Matches(name string) bool {
	tagName, err := tag.NewTagName(name)
	return err == nil && f.matcher.Matches(tagName)
}

// String renders the filter as its command-line flag.
func (f NameFilter) String() string {
	switch f.param {
	case apiv0.TagsQueryNamePattern:
		return fmt.Sprintf("--%s %q", patternFlag, f.expr)
	case apiv0.TagsQueryNameRegex:
		return fmt.Sprintf("--%s %q", regexFlag, f.expr)
	}
	return ""
}

func addNameFilterFlags(cmd *cobra.Command, pattern, regex *string, what string) {
	cmd.Flags().StringVar(pattern, patternFlag, "",
		what+` tags whose whole name matches a wildcard pattern ("*" any sequence, "?" one character)`)
	cmd.Flags().StringVar(regex, regexFlag, "",
		what+" tags whose name matches a Go (RE2) regular expression anywhere")
}

// nameFilterFromFlags builds the filter from --pattern/--regex. Giving both,
// or an invalid one, is a usage error (exit status 2).
func nameFilterFromFlags(cmd *cobra.Command, pattern, regex string) (NameFilter, error) {
	hasPattern := cmd.Flags().Changed(patternFlag)
	hasRegex := cmd.Flags().Changed(regexFlag)
	var (
		f   NameFilter
		err error
	)
	switch {
	case hasPattern && hasRegex:
		err = errors.New("use only one of --pattern and --regex")
	case hasPattern:
		f, err = NewPatternFilter(pattern)
	case hasRegex:
		f, err = NewRegexFilter(regex)
	}
	if err != nil {
		return NameFilter{}, usageError(err)
	}
	return f, nil
}
