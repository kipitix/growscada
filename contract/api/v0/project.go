package apiv0

import (
	"github.com/google/uuid"
)

// ProjectsQueryName is the filter of GET /projects selecting the project with
// this exact name, unique among the server's live projects.
const ProjectsQueryName = "name"

// ProjectResponse is the HTTP DTO for representing a project in API
// responses. The resources of a project live under /projects/{id}.
// Description is the project's optional, free-text description of at most 500
// characters (task 73); it is an attribute of the project, never part of its
// ProjectFile. HasUnsavedChanges reports whether the Draft moved since its
// last Commit.
type ProjectResponse struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Version           int       `json:"version"`
	HasUnsavedChanges bool      `json:"has_unsaved_changes"`
}

// GetProjectsResponse is the HTTP DTO for a list of projects.
type GetProjectsResponse struct {
	Projects []ProjectResponse `json:"projects"`
}

// CreateProjectRequest is the HTTP DTO for creating a project.
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateProjectResponse is the HTTP DTO for a project creation response.
// Version is the new project's version, the one its first rename states.
type CreateProjectResponse struct {
	ID      uuid.UUID `json:"id"`
	Version int       `json:"version"`
}

// UpdateProjectRequest is the HTTP DTO for updating a project's name and
// description. Version must match the current persisted version for optimistic
// locking. An absent Description keeps the current one, so a client that sends
// only a name does not erase it; an empty string clears it (task 73).
type UpdateProjectRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Version     int     `json:"version"`
}

// UpdateProjectResponse is the HTTP DTO for a project update response.
type UpdateProjectResponse struct {
	Version int `json:"version"`
}
