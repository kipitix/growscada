package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// ProjectCleanupPolicyName is the Name of the policy returned by
// NewProjectCleanupPolicy.
const ProjectCleanupPolicyName = "remove_deleted_project"

// projectCleanupPolicy empties a Project marked deleted and removes it
// (ADR 0006): its Scenes, then its Tags, each deleted in its own transaction
// with its own events, then the states of its Tags in the Runtime node
// (ADR 0011), and the Project's row last; a project_deleted event's job does
// it. Nothing of the Project is created once it is marked: the
// repositories lock its row, live, for a creation. For a live Project the
// job does nothing.
type projectCleanupPolicy struct {
	projects project.ProjectRepository
	scopes   ProjectScopes
	runtime  RuntimeNode
}

var _ Policy = (*projectCleanupPolicy)(nil)

// NewProjectCleanupPolicy returns the policy removing deleted Projects with
// their content.
func NewProjectCleanupPolicy(aProjectRepository project.ProjectRepository, someScopes ProjectScopes, aRuntime RuntimeNode) Policy {
	return &projectCleanupPolicy{projects: aProjectRepository, scopes: someScopes, runtime: aRuntime}
}

func (p projectCleanupPolicy) Name() string {
	return ProjectCleanupPolicyName
}

// Job key: "project:<id>".
func (p projectCleanupPolicy) JobKey(e event.Event) (string, bool) {
	switch e.Type() {
	case event.EventTypeProjectDeleted:
		if ev, ok := e.(project.ProjectEvent); ok {
			return projectKeyPrefix + ev.ProjectID().String(), true
		}
	}
	return "", false
}

// Run removes the Project of the job key, if it is marked deleted, with its
// content. A Scene or Tag that fails does not stop the others, but the
// Project stays until all are gone: Run returns the errors, and the job is
// run again. So it does when a Tag state is upserted after the states were
// removed (a creation's call, a bridge job running late): the row's removal
// fails while the state holds it, and the next run removes the state.
func (p projectCleanupPolicy) Run(ctx context.Context, key string) error {
	projectID, rest, err := cutProjectKey(key)
	if err != nil {
		return err
	}
	if rest != "" {
		return fmt.Errorf("invalid job key %q", key)
	}
	deleted, err := p.projects.FindDeleted(ctx, projectID)
	if errors.Is(err, project.ErrProjectNotFound) {
		return nil // live, or already removed
	}
	if err != nil {
		return fmt.Errorf("cannot find deleted project %s: %w", projectID, err)
	}

	scope := p.scopes.ForProject(projectID)
	if err := deleteAllScenes(ctx, scope.Scenes()); err != nil {
		return fmt.Errorf("cannot empty project %s: %w", projectID, err)
	}
	if err := deleteAllTags(ctx, scope.Tags()); err != nil {
		return fmt.Errorf("cannot empty project %s: %w", projectID, err)
	}
	if err := p.runtime.RemoveProjectTagStates(ctx, projectID.UUID()); err != nil {
		return fmt.Errorf("cannot empty project %s: %w", projectID, err)
	}
	if err := p.projects.Delete(ctx, deleted); err != nil && !errors.Is(err, project.ErrProjectNotFound) {
		return fmt.Errorf("cannot remove project %s: %w", projectID, err)
	}
	return nil
}

// deleteAllScenes deletes every Scene of repo, each in its own transaction;
// one deleted meanwhile is done.
func deleteAllScenes(ctx context.Context, repo scene.SceneRepository) error {
	scenes, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list scenes: %w", err)
	}
	var errs []error
	for _, sc := range scenes {
		if _, err := deleteScene(ctx, repo, sc.ID()); err != nil && !errors.Is(err, scene.ErrSceneNotFound) {
			errs = append(errs, fmt.Errorf("scene %s: %w", sc.ID(), err))
		}
	}
	return errors.Join(errs...)
}

// deleteAllTags deletes every Tag of repo, each in its own transaction; one
// deleted meanwhile is done.
func deleteAllTags(ctx context.Context, repo tag.TagRepository) error {
	tags, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list tags: %w", err)
	}
	var errs []error
	for _, t := range tags {
		if err := repo.Delete(ctx, t.Delete()); err != nil && !errors.Is(err, tag.ErrTagNotFound) {
			errs = append(errs, fmt.Errorf("tag %s: %w", t.ID(), err))
		}
	}
	return errors.Join(errs...)
}
