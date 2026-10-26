package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// RuntimeTagDefinitionsPolicyName is the Name of the policy returned by
// NewRuntimeTagDefinitionsPolicy.
const RuntimeTagDefinitionsPolicyName = "sync_runtime_tag_definitions"

// runtimeTagDefinitionsPolicy brings the Runtime node's copy of a Project's
// Tag definitions into line with the Draft (ADR 0011), Tag by Tag: a
// tag_created or tag_deleted event's job upserts the Tag's definition if the
// Draft holds it and removes its state if not. It is the bridge until Deploy
// exists (task 35), which then gives the Runtime node the definitions of the
// running Revision. The Tag creation and deletion do the same right after
// commit; the job makes up for a call that failed.
type runtimeTagDefinitionsPolicy struct {
	scopes  ProjectScopes
	runtime RuntimeNode
}

var _ Policy = (*runtimeTagDefinitionsPolicy)(nil)

// NewRuntimeTagDefinitionsPolicy returns the policy giving the Runtime node
// the Tag definitions of the Draft.
func NewRuntimeTagDefinitionsPolicy(someScopes ProjectScopes, aRuntime RuntimeNode) Policy {
	return &runtimeTagDefinitionsPolicy{scopes: someScopes, runtime: aRuntime}
}

func (p runtimeTagDefinitionsPolicy) Name() string {
	return RuntimeTagDefinitionsPolicyName
}

// Job key: "project:<id>/tag:<id>", the Tag the event names.
func (p runtimeTagDefinitionsPolicy) JobKey(e event.Event) (string, bool) {
	if e.Type() != event.EventTypeTagCreated && e.Type() != event.EventTypeTagDeleted {
		return "", false
	}
	if ev, ok := e.(tag.TagEvent); ok {
		return projectKeyPrefix + ev.ProjectID().String() + tagKeySeparator + ev.TagID().String(), true
	}
	return "", false
}

// Run reads the Tag of the job key from the Draft: if it exists, its
// definition is upserted (a state already there is kept), else its state is
// removed. Both are idempotent, so a job run late or twice gives the same
// result. In a Project marked deleted the Tag's state goes with its
// definition too: the Project's cleanup deletes the definitions, then every
// state left.
func (p runtimeTagDefinitionsPolicy) Run(ctx context.Context, key string) error {
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

	found, err := p.scopes.ForProject(projectID).Tags().FindByID(ctx, id.NewID(id.IDWithUUID[tag.Tag](tagUUID)))
	switch {
	case err == nil:
		if err := p.runtime.UpsertTagDefinitions(ctx, projectID.UUID(), []appdto.TagDefinition{appdto.NewTagDefinition(found)}); err != nil {
			return fmt.Errorf("cannot upsert tag definition %s: %w", tagUUID, err)
		}
	case errors.Is(err, tag.ErrTagNotFound):
		if err := p.runtime.RemoveTagStates(ctx, projectID.UUID(), []uuid.UUID{tagUUID}); err != nil {
			return fmt.Errorf("cannot remove tag state %s: %w", tagUUID, err)
		}
	default:
		return fmt.Errorf("cannot find tag %s: %w", tagUUID, err)
	}
	return nil
}
