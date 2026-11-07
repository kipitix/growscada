package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// ProjectCleanupPolicyName is the Name of the policy returned by
// NewProjectCleanupPolicy.
const ProjectCleanupPolicyName = "remove_deleted_project"

// projectCleanupPolicy empties a Project marked deleted and removes it
// (ADR 0006): its whole Draft content — Scenes, Tags, EnvironmentDevices,
// Devices, Environments and Library Pins — in one transaction over the Draft
// with the events of the removal, then the states of its Tags in the Runtime
// node (ADR 0011), and the Project's row last; a project_deleted event's job
// does it. The Runtime node's copies of the Project's Devices and
// Environments are derived data and go with its row (their foreign keys
// cascade), while its Tag copies are removed before it through RuntimeNode,
// and its Device Tokens are forgotten through it after the row went (they
// cascade, but a Gateway must not keep authenticating with one). For a live
// Project the job does nothing.
type projectCleanupPolicy struct {
	projects project.ProjectRepository
	drafts   draft.Repository
	runtime  RuntimeNode
}

var _ Policy = (*projectCleanupPolicy)(nil)

// NewProjectCleanupPolicy returns the policy removing deleted Projects with
// their content.
func NewProjectCleanupPolicy(aProjectRepository project.ProjectRepository, someDrafts draft.Repository, aRuntime RuntimeNode) Policy {
	return &projectCleanupPolicy{projects: aProjectRepository, drafts: someDrafts, runtime: aRuntime}
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
// content. The Project stays until its content is gone: Run returns an error
// and the job is run again. So it does when a Tag state is upserted after the
// states were removed (a creation's call, a bridge job running late): the
// row's removal fails while the state holds it, and the next run removes the
// state.
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

	if _, err := p.drafts.Purge(ctx, draft.ProjectScope(projectID)); err != nil {
		return fmt.Errorf("cannot empty project %s: %w", projectID, err)
	}
	if err := p.runtime.RemoveProjectTagStates(ctx, projectID.UUID()); err != nil {
		return fmt.Errorf("cannot empty project %s: %w", projectID, err)
	}
	if err := p.projects.Delete(ctx, deleted); err != nil && !errors.Is(err, project.ErrProjectNotFound) {
		return fmt.Errorf("cannot remove project %s: %w", projectID, err)
	}
	// The Project's token rows went with its row (the foreign key cascades),
	// but the Runtime node may hold the tokens it resolved in memory: a token
	// of the removed Project must not authenticate any more.
	if err := p.runtime.RemoveProjectDeviceTokens(ctx, projectID.UUID()); err != nil {
		return fmt.Errorf("cannot remove project %s: %w", projectID, err)
	}
	return nil
}
