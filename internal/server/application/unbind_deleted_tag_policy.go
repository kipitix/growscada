package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// UnbindDeletedTagPolicyName is the Name of the policy returned by
// NewUnbindDeletedTagPolicy.
const UnbindDeletedTagPolicyName = "unbind_deleted_tag"

// unbindDeletedTagPolicy removes, from Widgets of every WidgetType, the
// PortBindings of a deleted Tag (ADR 0010): a tag_deleted event's job unbinds
// the Tag in every Scene bound to it, once it has read that the Tag is gone.
// A Widget bound to the Tag concurrently with its deletion, too late for the
// job to find, is unbound by the job of its own event
// (widgetReconciliationPolicy). The events of the policy's own unbinding bind
// nothing and enqueue no job.
type unbindDeletedTagPolicy struct {
	scenes scene.SceneRepository
	tags   tag.TagRepository
}

var _ Policy = (*unbindDeletedTagPolicy)(nil)

// NewUnbindDeletedTagPolicy returns the policy removing the PortBindings of
// deleted Tags.
func NewUnbindDeletedTagPolicy(aSceneRepository scene.SceneRepository, aTagRepository tag.TagRepository) Policy {
	return &unbindDeletedTagPolicy{scenes: aSceneRepository, tags: aTagRepository}
}

func (p unbindDeletedTagPolicy) Name() string {
	return UnbindDeletedTagPolicyName
}

// Job key: "tag:<id>" for every Scene bound to the Tag.
const tagKeyPrefix = "tag:"

func (p unbindDeletedTagPolicy) JobKey(e event.Event) (string, bool) {
	if e.Type() != event.EventTypeTagDeleted {
		return "", false
	}
	if ev, ok := e.(tag.TagEvent); ok {
		return tagKeyPrefix + ev.TagID().String(), true
	}
	return "", false
}

// Run unbinds the Tag of the job key in every Scene bound to it, each in its
// own transaction, if the Tag does not exist now: a Tag with that ID created
// again since the event keeps its bindings. A Scene that fails does not stop
// the others: Run returns the errors of all that failed, and the job is run
// again, the Scenes already unbound then no longer found.
func (p unbindDeletedTagPolicy) Run(ctx context.Context, key string) error {
	rawTagID, ok := strings.CutPrefix(key, tagKeyPrefix)
	if !ok {
		return fmt.Errorf("invalid job key %q", key)
	}
	tagUUID, err := uuid.Parse(rawTagID)
	if err != nil {
		return fmt.Errorf("invalid tag ID in job key %q: %w", key, err)
	}
	tagID := id.NewID(id.IDWithUUID[tag.Tag](tagUUID))

	_, err = p.tags.FindByID(ctx, tagID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, tag.ErrTagNotFound) {
		return fmt.Errorf("cannot find tag %s: %w", tagID, err)
	}

	scenes, err := p.scenes.FindByTagID(ctx, tagID)
	if err != nil {
		return fmt.Errorf("cannot list scenes bound to tag %s: %w", tagID, err)
	}
	err = changeEachScene(ctx, p.scenes, scenes, func(_ context.Context, sc scene.Scene) (scene.Scene, bool, error) {
		unbound, changed := sc.UnbindTag(tagID)
		return unbound, changed, nil
	})
	if err != nil {
		return fmt.Errorf("cannot unbind tag %s: %w", tagID, err)
	}
	return nil
}
