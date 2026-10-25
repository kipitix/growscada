package apiv0

import (
	"github.com/google/uuid"
)

// ProjectsQueryName is the filter of GET /projects selecting the project with
// this exact name, unique among the server's live projects.
const ProjectsQueryName = "name"

// ProjectResponse is the HTTP DTO for representing a project in API
// responses. The resources of a project live under /projects/{id}.
type ProjectResponse struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Version int       `json:"version"`
}

// GetProjectsResponse is the HTTP DTO for a list of projects.
type GetProjectsResponse struct {
	Projects []ProjectResponse `json:"projects"`
}

// CreateProjectRequest is the HTTP DTO for creating a project.
type CreateProjectRequest struct {
	Name string `json:"name"`
}

// CreateProjectResponse is the HTTP DTO for a project creation response.
// Version is the new project's version, the one its first rename states.
type CreateProjectResponse struct {
	ID      uuid.UUID `json:"id"`
	Version int       `json:"version"`
}

// UpdateProjectRequest is the HTTP DTO for renaming a project.
// Version must match the current persisted version for optimistic locking.
type UpdateProjectRequest struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// UpdateProjectResponse is the HTTP DTO for a project rename response.
type UpdateProjectResponse struct {
	Version int `json:"version"`
}
