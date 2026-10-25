package project

import "fmt"

// ProjectName - name of a project, unique among the live Projects of its
// Organization (ADR 0006).
// Value Object.
type ProjectName struct {
	name string
}

var _ fmt.Stringer = ProjectName{}

// NewProjectName creates a ProjectName from a string.
func NewProjectName(aName string) (ProjectName, error) {
	if aName == "" {
		return ProjectName{}, fmt.Errorf("project name must not be empty")
	}
	return ProjectName{name: aName}, nil
}

// String implements [fmt.Stringer].
func (n ProjectName) String() string { return n.name }
