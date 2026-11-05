package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// ProjectContent opens the content of the live Projects of one Organization
// for the services: every access to a Project's content first checks the
// Project (ADR 0006), so a Project marked deleted, or one of another
// Organization, has no content for the API even while its cleanup runs. The
// content is the Project's Draft (ADR 0016): reads go through the Project's
// scope, writes through the Draft's repository.
type ProjectContent struct {
	projects     project.ProjectRepository
	scopes       ProjectScopes
	drafts       draft.Repository
	organization id.ID[organization.Organization]
}

// NewProjectContent returns the ProjectContent of the Projects of the
// Organization anOrganizationID, its reads through someScopes and its writes
// through the Drafts.
func NewProjectContent(aProjectRepository project.ProjectRepository, someScopes ProjectScopes, someDrafts draft.Repository, anOrganizationID id.ID[organization.Organization]) ProjectContent {
	return ProjectContent{projects: aProjectRepository, scopes: someScopes, drafts: someDrafts, organization: anOrganizationID}
}

// Drafts is the repository of the Drafts of every Project.
func (c ProjectContent) Drafts() draft.Repository { return c.drafts }

// ScopeOf is the scope of the Draft of projectID.
func (c ProjectContent) ScopeOf(projectID id.ID[project.Project]) editlock.Scope {
	return editlock.ProjectScope(projectID)
}

// open returns the ID and the read repositories of the live Project
// rawProjectID. Returns an error wrapping project.ErrProjectNotFound if there
// is none.
func (c ProjectContent) open(ctx context.Context, rawProjectID uuid.UUID) (id.ID[project.Project], ProjectScope, error) {
	projectID := id.NewID(id.IDWithUUID[project.Project](rawProjectID))
	if _, err := findOwnProject(ctx, c.projects, c.organization, projectID); err != nil {
		return projectID, nil, err
	}
	return projectID, c.scopes.ForProject(projectID), nil
}

// openUnchecked returns the ID and the read repositories of the Project
// rawProjectID without checking it: for a query that checks the Project
// itself (and open on a miss, for the error).
func (c ProjectContent) openUnchecked(rawProjectID uuid.UUID) (id.ID[project.Project], ProjectScope) {
	projectID := id.NewID(id.IDWithUUID[project.Project](rawProjectID))
	return projectID, c.scopes.ForProject(projectID)
}

// Version returns the Version of the Draft of the live Project rawProjectID:
// the number every content DTO of that Project carries. A Draft never written
// has version 0.
func (c ProjectContent) Version(ctx context.Context, rawProjectID uuid.UUID) (int, error) {
	projectID, _ := c.openUnchecked(rawProjectID)
	found, ok, err := c.drafts.Read(ctx, editlock.ProjectScope(projectID))
	if err != nil {
		return 0, fmt.Errorf("cannot read the draft of project %s: %w", projectID, err)
	}
	if !ok {
		// A Draft whose row does not exist yet is a fresh Draft at version 1:
		// the Version its first write states and its row starts at.
		return 1, nil
	}
	return found.Version().Number(), nil
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
