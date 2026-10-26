package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// UnbindDeletedTagPolicyName is the Name of the policy returned by
// NewUnbindDeletedTagPolicy.
const UnbindDeletedTagPolicyName = "unbind_deleted_tag"

// unbindDeletedTagPolicy removes, from Widgets of every WidgetType, the
// PortBindings of a deleted Tag (ADR 0010): a tag_deleted event's job unbinds
// the Tag in every Scene of its Project bound to it, once it has read that
// the Tag is gone.
// A Widget bound to the Tag concurrently with its deletion, too late for the
// job to find, is unbound by the job of its own event
// (widgetReconciliationPolicy). The events of the policy's own unbinding bind
// nothing and enqueue no job. In a Project marked deleted it unbinds
// nothing: the Project's cleanup deletes its Scenes, before the Tags whose
// tag_deleted it then keys a job for each.
type unbindDeletedTagPolicy struct {
	projects project.ProjectRepository
	scopes   ProjectScopes
}

var _ Policy = (*unbindDeletedTagPolicy)(nil)

// NewUnbindDeletedTagPolicy returns the policy removing the PortBindings of
// deleted Tags. It works through the repositories of the event's Project,
// read from projects whether it is live.
func NewUnbindDeletedTagPolicy(aProjectRepository project.ProjectRepository, someScopes ProjectScopes) Policy {
	return &unbindDeletedTagPolicy{projects: aProjectRepository, scopes: someScopes}
}

func (p unbindDeletedTagPolicy) Name() string {
	return UnbindDeletedTagPolicyName
}

// Job key: "project:<id>/tag:<id>" for every Scene of the Project bound to
// the Tag.
const tagKeySeparator = "/tag:"

func (p unbindDeletedTagPolicy) JobKey(e event.Event) (string, bool) {
	if e.Type() != event.EventTypeTagDeleted {
		return "", false
	}
	if ev, ok := e.(tag.TagEvent); ok {
		return projectKeyPrefix + ev.ProjectID().String() + tagKeySeparator + ev.TagID().String(), true
	}
	return "", false
}

// Run unbinds the Tag of the job key in every Scene bound to it, each in its
// own transaction, if the Tag does not exist now and its Project is live: a
// Tag with that ID created again since the event keeps its bindings, and
// the Scenes of a Project marked deleted (or removed) are its cleanup's. A
// Scene that fails does not stop
// the others: Run returns the errors of all that failed, and the job is run
// again, the Scenes already unbound then no longer found.
func (p unbindDeletedTagPolicy) Run(ctx context.Context, key string) error {
	projectID, rest, err := cutProjectKey(key)
	if err != nil {
		return err
	}
	rawTagID, ok := strings.CutPrefix(rest, tagKeySeparator)
	if !ok {
		return fmt.Errorf("invalid job key %q", key)
	}
	tagUUID, err := uuid.Parse(rawTagID)
	if err != nil {
		return fmt.Errorf("invalid tag ID in job key %q: %w", key, err)
	}
	tagID := id.NewID(id.IDWithUUID[tag.Tag](tagUUID))
	scope := p.scopes.ForProject(projectID)

	_, err = scope.Tags().FindByID(ctx, tagID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, tag.ErrTagNotFound) {
		return fmt.Errorf("cannot find tag %s: %w", tagID, err)
	}
	if _, err := p.projects.FindByID(ctx, projectID); errors.Is(err, project.ErrProjectNotFound) {
		return nil
	} else if err != nil {
		return fmt.Errorf("cannot find project %s: %w", projectID, err)
	}

	scenes, err := scope.Scenes().FindByTagID(ctx, tagID)
	if err != nil {
		return fmt.Errorf("cannot list scenes bound to tag %s: %w", tagID, err)
	}
	err = changeEachScene(ctx, scope.Scenes(), scenes, func(_ context.Context, sc scene.Scene) (scene.Scene, bool, error) {
		unbound, changed := sc.UnbindTag(tagID)
		return unbound, changed, nil
	})
	if err != nil {
		return fmt.Errorf("cannot unbind tag %s: %w", tagID, err)
	}
	return nil
}
