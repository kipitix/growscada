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
// the Tag in every Scene bound to it. A Widget bound to the Tag concurrently
// with its deletion, too late for the job to find, is unbound by the job of
// its own event (widgetReconciliationPolicy). The events of the policy's own
// unbinding bind nothing and enqueue no job.
type unbindDeletedTagPolicy struct {
	scenes scene.SceneRepository
}

var _ Policy = (*unbindDeletedTagPolicy)(nil)

// NewUnbindDeletedTagPolicy returns the policy removing the PortBindings of
// deleted Tags.
func NewUnbindDeletedTagPolicy(aSceneRepository scene.SceneRepository) Policy {
	return &unbindDeletedTagPolicy{scenes: aSceneRepository}
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
// own transaction. A Scene that fails does not stop the others: Run returns
// the errors of all that failed, and the job is run again, the Scenes
// already unbound then no longer found.
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

	scenes, err := p.scenes.FindByTagID(ctx, tagID)
	if err != nil {
		return fmt.Errorf("cannot list scenes bound to tag %s: %w", tagID, err)
	}

	var errs []error
	for _, sc := range scenes {
		if err := p.unbindInScene(ctx, sc, tagID); err != nil {
			errs = append(errs, fmt.Errorf("cannot unbind tag %s in scene %s: %w", tagID, sc.ID(), err))
		}
	}
	return errors.Join(errs...)
}

// unbindInScene unbinds the Tag tagID in the Scene sc and saves it if
// anything changed. Like a reconciliation it does not rest on what a client
// saw: a Scene changed since it was read is reread and unbound again
// (retryOnRace). A Scene deleted meanwhile leaves nothing to unbind.
func (p unbindDeletedTagPolicy) unbindInScene(ctx context.Context, sc scene.Scene, tagID id.ID[tag.Tag]) error {
	first := true
	err := retryOnRace(scene.ErrSceneConflict, func() error {
		if !first {
			var err error
			if sc, err = p.scenes.FindByID(ctx, sc.ID()); err != nil {
				return fmt.Errorf("cannot reread scene: %w", err)
			}
		}
		first = false
		unbound, changed := sc.UnbindTag(tagID)
		if !changed {
			return nil
		}
		_, err := p.scenes.Save(ctx, unbound)
		return err
	})
	if errors.Is(err, scene.ErrSceneNotFound) {
		return nil
	}
	return err
}
