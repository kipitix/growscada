package tag

import (
	"errors"
	"regexp"
)

// TagName is a value object for tag name
type TagName interface {
	Name() string
	Equals(other TagName) bool
	String() string
}

type tagName struct {
	name string
}

var _ TagName = (*tagName)(nil)

func NewTagName(name string) (TagName, error) {
	if len(name) == 0 {
		return nil, errors.New("tag name cannot be empty")
	}

	if len(name) > 100 {
		return nil, errors.New("tag name cannot exceed 100 characters")
	}

	// Allow only alphanumeric characters, underscores, hyphens, and periods
	matched, _ := regexp.MatchString("^[a-zA-Z0-9_.-]+$", name)
	if !matched {
		return nil, errors.New("tag name contains invalid characters")
	}

	return &tagName{name: name}, nil
}

func (tn tagName) Name() string {
	return tn.name
}

func (tn tagName) Equals(other TagName) bool {
	return tn.name == other.Name()
}

func (tn tagName) String() string {
	return tn.name
}
