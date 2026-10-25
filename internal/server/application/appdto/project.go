package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// Project is the application-layer DTO for project data. Its Organization is
// left out: it is the server's own, the same for every project.
type Project struct {
	ID      uuid.UUID
	Name    string
	Version int
}

// ProjectInput holds the fields of a project a client creates or renames.
// The project's ID and the version a rename expects are passed separately.
type ProjectInput struct {
	Name string
}

// NewProject creates a Project DTO from the domain aggregate.
func NewProject(p project.Project) Project {
	return Project{
		ID:      p.ID().UUID(),
		Name:    p.Name().String(),
		Version: p.Version().Number(),
	}
}

// NewProjectList creates a slice of Project DTOs from domain aggregates.
func NewProjectList(list []project.Project) []Project {
	result := make([]Project, len(list))
	for i, p := range list {
		result[i] = NewProject(p)
	}
	return result
}
