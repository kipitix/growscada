package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// ProjectService is the service interface for working with the projects of
// the server's Organization.
type ProjectService interface {
	FindAllProjects(context.Context) ([]appdto.Project, error)
	FindProjectByID(context.Context, uuid.UUID) (appdto.Project, error)
	FindProjectByName(context.Context, string) (appdto.Project, error)
	CreateProject(context.Context, appdto.ProjectInput) (appdto.Project, error)
	UpdateProject(ctx context.Context, projectID uuid.UUID, version int, input appdto.ProjectInput) (appdto.Project, error)
	DeleteProjectByID(context.Context, uuid.UUID) (appdto.Project, error)
}

type projectServiceImpl struct {
	repository   project.ProjectRepository
	organization id.ID[organization.Organization]
}

var _ ProjectService = (*projectServiceImpl)(nil)

// NewProjectService returns the service of the projects of the Organization
// anOrganizationID. The events of its changes are recorded by the Project and
// saved with it (ADR 0008); the content of a deleted project is removed by
// the policy of NewProjectCleanupPolicy.
func NewProjectService(aRepository project.ProjectRepository, anOrganizationID id.ID[organization.Organization]) ProjectService {
	return &projectServiceImpl{repository: aRepository, organization: anOrganizationID}
}

func (s projectServiceImpl) FindAllProjects(ctx context.Context) ([]appdto.Project, error) {
	list, err := s.repository.FindAll(ctx, s.organization)
	if err != nil {
		return nil, fmt.Errorf("error on find projects in repository: %w", err)
	}
	return appdto.NewProjectList(list), nil
}

func (s projectServiceImpl) FindProjectByID(ctx context.Context, rawID uuid.UUID) (appdto.Project, error) {
	found, err := findOwnProject(ctx, s.repository, s.organization, id.NewID(id.IDWithUUID[project.Project](rawID)))
	if err != nil {
		return appdto.Project{}, err
	}
	return appdto.NewProject(found), nil
}

// FindProjectByName returns the live project with the name, unique in the
// Organization.
func (s projectServiceImpl) FindProjectByName(ctx context.Context, rawName string) (appdto.Project, error) {
	name, err := project.NewProjectName(rawName)
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot find project because of name: %w", invalidInput(err))
	}
	found, err := s.repository.FindByName(ctx, s.organization, name)
	if err != nil {
		return appdto.Project{}, fmt.Errorf("error on find project by name in repository: %w", err)
	}
	return appdto.NewProject(found), nil
}

func (s projectServiceImpl) CreateProject(ctx context.Context, input appdto.ProjectInput) (appdto.Project, error) {
	name, err := project.NewProjectName(input.Name)
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot create project: name: %w", invalidInput(err))
	}
	saved, err := s.repository.Save(ctx, project.CreateProject(s.repository.NextID(), s.organization, name))
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot save project: %w", err)
	}
	return appdto.NewProject(saved), nil
}

// UpdateProject renames a project, expecting its Version.
func (s projectServiceImpl) UpdateProject(ctx context.Context, rawID uuid.UUID, rawExpectedVersion int, input appdto.ProjectInput) (appdto.Project, error) {
	found, err := findOwnProject(ctx, s.repository, s.organization, id.NewID(id.IDWithUUID[project.Project](rawID)))
	if err != nil {
		return appdto.Project{}, err
	}

	// A negative version is no version at all: the client's input, not a conflict.
	expected, err := version.New(version.WithNumber[project.Project](rawExpectedVersion))
	if err != nil {
		return appdto.Project{}, invalidInput(fmt.Errorf("version: %w", err))
	}
	// A stale write is a conflict whatever it sends.
	if err := found.CheckVersion(expected); err != nil {
		return appdto.Project{}, fmt.Errorf("cannot update project: %w", err)
	}
	name, err := project.NewProjectName(input.Name)
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot update project: name: %w", invalidInput(err))
	}
	renamed, err := found.Rename(expected, name)
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot update project: %w", err)
	}

	saved, err := s.repository.Save(ctx, renamed)
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot save project: %w", err)
	}
	return appdto.NewProject(saved), nil
}

// DeleteProjectByID marks a project deleted and returns it as it was read:
// from then on it does not exist for the API, and its name is free. Its
// content is removed later, by the policy of NewProjectCleanupPolicy
// (ADR 0006). It expects no Version: a project renamed meanwhile is reread
// and deleted as it is now (retryOnRace).
func (s projectServiceImpl) DeleteProjectByID(ctx context.Context, rawID uuid.UUID) (appdto.Project, error) {
	projectID := id.NewID(id.IDWithUUID[project.Project](rawID))

	var deleted project.Project
	err := retryOnRace(project.ErrProjectConflict, func() error {
		found, err := findOwnProject(ctx, s.repository, s.organization, projectID)
		if err != nil {
			return err
		}
		if _, err := s.repository.Save(ctx, found.Delete()); err != nil {
			return fmt.Errorf("cannot delete project: %w", err)
		}
		deleted = found
		return nil
	})
	if err != nil {
		return appdto.Project{}, err
	}
	return appdto.NewProject(deleted), nil
}
