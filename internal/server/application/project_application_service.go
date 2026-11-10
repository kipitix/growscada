package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/description"
	"github.com/kipitix/growscada/internal/server/domain/draft"
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
	drafts       draft.Repository
	organization id.ID[organization.Organization]
}

var _ ProjectService = (*projectServiceImpl)(nil)

// NewProjectService returns the service of the projects of the Organization
// anOrganizationID. The events of its changes are recorded by the Project and
// saved with it (ADR 0008); the content of a deleted project is removed by
// the policy of NewProjectCleanupPolicy. The Drafts are read for the
// unsaved-changes flag of a project's selection row (task 73).
func NewProjectService(aRepository project.ProjectRepository, someDrafts draft.Repository, anOrganizationID id.ID[organization.Organization]) ProjectService {
	return &projectServiceImpl{repository: aRepository, drafts: someDrafts, organization: anOrganizationID}
}

func (s projectServiceImpl) FindAllProjects(ctx context.Context) ([]appdto.Project, error) {
	list, err := s.repository.FindAll(ctx, s.organization)
	if err != nil {
		return nil, fmt.Errorf("error on find projects in repository: %w", err)
	}
	states := make([]appdto.ProjectDraftState, len(list))
	for i, p := range list {
		state, err := s.draftState(ctx, p.ID())
		if err != nil {
			return nil, err
		}
		states[i] = state
	}
	return appdto.NewProjectList(list, states), nil
}

func (s projectServiceImpl) FindProjectByID(ctx context.Context, rawID uuid.UUID) (appdto.Project, error) {
	found, err := findOwnProject(ctx, s.repository, s.organization, id.NewID(id.IDWithUUID[project.Project](rawID)))
	if err != nil {
		return appdto.Project{}, err
	}
	return s.describe(ctx, found)
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
	return s.describe(ctx, found)
}

func (s projectServiceImpl) CreateProject(ctx context.Context, input appdto.ProjectInput) (appdto.Project, error) {
	name, err := project.NewProjectName(input.Name)
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot create project: name: %w", invalidInput(err))
	}
	desc, err := descriptionOf(input.Description, description.Description{})
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot create project: %w", err)
	}
	saved, err := s.repository.Save(ctx, project.CreateProject(s.repository.NextID(), s.organization, name, desc))
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot save project: %w", err)
	}
	return s.describe(ctx, saved)
}

// UpdateProject changes a project's name and description, expecting its
// Version. A nil description keeps the current one (task 73).
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
	desc, err := descriptionOf(input.Description, found.Description())
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot update project: %w", err)
	}
	updated, err := found.Update(expected, name, desc)
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot update project: %w", err)
	}

	saved, err := s.repository.Save(ctx, updated)
	if err != nil {
		return appdto.Project{}, fmt.Errorf("cannot save project: %w", err)
	}
	return s.describe(ctx, saved)
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
	// The project is gone: its Draft tells nothing of a deleted project.
	return appdto.NewProject(deleted, appdto.ProjectDraftState{}), nil
}

// describe returns the DTO of p with the state of its Draft.
func (s projectServiceImpl) describe(ctx context.Context, p project.Project) (appdto.Project, error) {
	state, err := s.draftState(ctx, p.ID())
	if err != nil {
		return appdto.Project{}, err
	}
	return appdto.NewProject(p, state), nil
}

// draftState reads the Version of a project's Draft and its unsaved-changes
// flag, both from the Draft's row alone: the flag is the Draft's own Clean
// Version marker (ADR 0016, amendment 2026-11-10). A Draft never written has
// nothing to save: its state is the zero one, so the project shows no unsaved
// changes (task 73, task 74).
func (s projectServiceImpl) draftState(ctx context.Context, projectID id.ID[project.Project]) (appdto.ProjectDraftState, error) {
	foundDraft, ok, err := s.drafts.Read(ctx, draft.ProjectScope(projectID))
	if err != nil {
		return appdto.ProjectDraftState{}, fmt.Errorf("cannot read the draft of project %s: %w", projectID, err)
	}
	if !ok {
		return appdto.ProjectDraftState{}, nil
	}
	return appdto.ProjectDraftState{
		DraftVersion:      foundDraft.Version().Number(),
		HasUnsavedChanges: foundDraft.HasUnsavedChanges(),
	}, nil
}
