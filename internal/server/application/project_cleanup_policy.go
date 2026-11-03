package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/device"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// ProjectCleanupPolicyName is the Name of the policy returned by
// NewProjectCleanupPolicy.
const ProjectCleanupPolicyName = "remove_deleted_project"

// projectCleanupPolicy empties a Project marked deleted and removes it
// (ADR 0006): its Scenes, then its Tags, its EnvironmentDevices, its Devices
// and its Environments, and its Library Pins, each deleted in its own
// transaction with its own events, then the states of its Tags in the Runtime
// node (ADR 0011), and the Project's row last; a project_deleted event's job
// does it. The Runtime node's copies of the Project's Devices and
// Environments are derived data and go with its row (their foreign keys
// cascade), while its Tag copies are removed before it through RuntimeNode,
// and its Device Tokens are forgotten through it after the row went (they
// cascade, but a Gateway must not keep authenticating with one). Nothing of
// the Project is created once it is marked: the repositories lock its row,
// live, for a creation. For a live Project the job does nothing.
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
	if err := deleteAllEnvironmentDevices(ctx, scope.EnvironmentDevices()); err != nil {
		return fmt.Errorf("cannot empty project %s: %w", projectID, err)
	}
	if err := deleteAllDevices(ctx, scope.Devices()); err != nil {
		return fmt.Errorf("cannot empty project %s: %w", projectID, err)
	}
	if err := deleteAllEnvironments(ctx, scope.Environments()); err != nil {
		return fmt.Errorf("cannot empty project %s: %w", projectID, err)
	}
	if err := deleteAllPins(ctx, scope.LibraryPins()); err != nil {
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

// deleteAllEnvironmentDevices deletes every pair of repo, each in its own
// transaction; one deleted meanwhile is done. The pairs go before the Devices
// and the Environments they hold.
func deleteAllEnvironmentDevices(ctx context.Context, repo device.EnvironmentDeviceRepository) error {
	pairs, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list environment devices: %w", err)
	}
	var errs []error
	for _, pair := range pairs {
		if err := repo.Delete(ctx, pair.Delete()); err != nil && !errors.Is(err, device.ErrEnvironmentDeviceNotFound) {
			errs = append(errs, fmt.Errorf("environment %s device %s: %w", pair.EnvironmentID(), pair.DeviceID(), err))
		}
	}
	return errors.Join(errs...)
}

// deleteAllDevices deletes every Device of repo, each in its own transaction;
// one deleted meanwhile is done. The Tags and the pairs are gone already, so
// nothing refers to a Device.
func deleteAllDevices(ctx context.Context, repo device.DeviceRepository) error {
	devices, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list devices: %w", err)
	}
	var errs []error
	for _, d := range devices {
		if err := repo.Delete(ctx, d.Delete()); err != nil && !errors.Is(err, device.ErrDeviceNotFound) {
			errs = append(errs, fmt.Errorf("device %s: %w", d.ID(), err))
		}
	}
	return errors.Join(errs...)
}

// deleteAllEnvironments deletes every Environment of repo, each in its own
// transaction; one deleted meanwhile is done. The pairs are gone already, so
// no Environment holds a Device.
func deleteAllEnvironments(ctx context.Context, repo device.EnvironmentRepository) error {
	environments, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list environments: %w", err)
	}
	var errs []error
	for _, e := range environments {
		if err := repo.Delete(ctx, e.Delete()); err != nil && !errors.Is(err, device.ErrEnvironmentNotFound) {
			errs = append(errs, fmt.Errorf("environment %s: %w", e.ID(), err))
		}
	}
	return errors.Join(errs...)
}

// deleteAllPins deletes every Library Pin of repo, each in its own
// transaction; one deleted meanwhile is done. The Scenes are gone already,
// so no Widget is left to check.
func deleteAllPins(ctx context.Context, repo librarypin.Repository) error {
	pins, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot list library pins: %w", err)
	}
	var errs []error
	for _, pin := range pins {
		if err := repo.Delete(ctx, pin.Delete(), nil); err != nil && !errors.Is(err, librarypin.ErrLibraryPinNotFound) {
			errs = append(errs, fmt.Errorf("library pin of %s: %w", pin.LibraryID(), err))
		}
	}
	return errors.Join(errs...)
}
