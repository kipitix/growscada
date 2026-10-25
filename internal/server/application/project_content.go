package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// ProjectContent opens the content of the live Projects of one Organization
// for the services: every access to a Project's content first checks the
// Project (ADR 0006), so a Project marked deleted, or one of another
// Organization, has no content for the API even while its cleanup runs.
type ProjectContent struct {
	projects     project.ProjectRepository
	scopes       ProjectScopes
	organization id.ID[organization.Organization]
}

// NewProjectContent returns the ProjectContent of the Projects of the
// Organization anOrganizationID.
func NewProjectContent(aProjectRepository project.ProjectRepository, someScopes ProjectScopes, anOrganizationID id.ID[organization.Organization]) ProjectContent {
	return ProjectContent{projects: aProjectRepository, scopes: someScopes, organization: anOrganizationID}
}

// open returns the ID and the repositories of the live Project rawProjectID.
// Returns an error wrapping project.ErrProjectNotFound if there is none.
func (c ProjectContent) open(ctx context.Context, rawProjectID uuid.UUID) (id.ID[project.Project], ProjectScope, error) {
	projectID := id.NewID(id.IDWithUUID[project.Project](rawProjectID))
	if _, err := findOwnProject(ctx, c.projects, c.organization, projectID); err != nil {
		return projectID, nil, err
	}
	return projectID, c.scopes.ForProject(projectID), nil
}

// findOwnProject returns the live Project projectID of the Organization
// orgID; one of another Organization is not found.
func findOwnProject(ctx context.Context, projects project.ProjectRepository, orgID id.ID[organization.Organization], projectID id.ID[project.Project]) (project.Project, error) {
	found, err := projects.FindByID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("error on find project by id in repository: %w", err)
	}
	if found.OrganizationID() != orgID {
		return nil, id.NotFound(project.ErrProjectNotFound, projectID)
	}
	return found, nil
}
