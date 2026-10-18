package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// WidgetReconciliationPolicyName is the Name of the policy returned by
// NewWidgetReconciliationPolicy.
const WidgetReconciliationPolicyName = "reconcile_widgets_with_widget_type"

// widgetReconciliationPolicy removes, from Widgets of a WidgetType, the
// PortBindings that are no longer valid (ADR 0010): of ports the WidgetType
// no longer declares, of Tags whose TagType a port's type hint no longer
// accepts and of Tags that no longer exist. A widget_type_updated event may
// have dropped a port or narrowed a type hint: its job reconciles every Scene
// holding Widgets of the type. A widget_created or widget_updated event may
// have bound a port that a concurrent update of the WidgetType had just
// dropped or narrowed, or a Tag deleted concurrently (the Widget was checked
// against the WidgetType and the Tags as read before that change): its job
// reconciles only the Scene of the Widget. The events of the policy's own
// reconciliations bind nothing and enqueue no job.
//
// It is transitional: once a Project sees only a Library Release (ADR 0007),
// reconciliation moves to the DraftChange of a pinned release (task 33).
type widgetReconciliationPolicy struct {
	widgetTypes library.WidgetTypeRepository
	scenes      scene.SceneRepository
	tags        tag.TagRepository
}

var _ Policy = (*widgetReconciliationPolicy)(nil)

// NewWidgetReconciliationPolicy returns the policy bringing Widgets into line
// with their WidgetType.
func NewWidgetReconciliationPolicy(aWidgetTypeRepository library.WidgetTypeRepository, aSceneRepository scene.SceneRepository, aTagRepository tag.TagRepository) Policy {
	return &widgetReconciliationPolicy{
		widgetTypes: aWidgetTypeRepository,
		scenes:      aSceneRepository,
		tags:        aTagRepository,
	}
}

func (p widgetReconciliationPolicy) Name() string {
	return WidgetReconciliationPolicyName
}

// Job keys: "widget_type:<id>" for every Scene of a WidgetType,
// "widget_type:<id>/scene:<id>" for one Scene.
const (
	widgetTypeKeyPrefix = "widget_type:"
	sceneKeySeparator   = "/scene:"
)

func (p widgetReconciliationPolicy) JobKey(e event.Event) (string, bool) {
	switch e.Type() {
	case event.EventTypeWidgetTypeUpdated:
		if ev, ok := e.(library.WidgetTypeEvent); ok {
			return widgetTypeKeyPrefix + ev.WidgetTypeID().String(), true
		}
	case event.EventTypeWidgetCreated, event.EventTypeWidgetUpdated:
		if ev, ok := e.(scene.WidgetUpdatedEvent); ok && ev.ByReconciliation() {
			return "", false
		}
		if ev, ok := e.(scene.WidgetTypedEvent); ok {
			return widgetTypeKeyPrefix + ev.TypeID().String() + sceneKeySeparator + ev.SceneID().String(), true
		}
	}
	return "", false
}

// Run reconciles the Scenes of the job key, each in its own transaction, with
// the WidgetType as it is now: every Scene holding Widgets of the WidgetType,
// or the one Scene the key names. A Scene that fails does not stop the
// others: Run returns the errors of all that failed, and the job is run
// again, the Scenes already reconciled then left as they are. A deleted
// WidgetType or Scene has no Widgets left to reconcile.
func (p widgetReconciliationPolicy) Run(ctx context.Context, key string) error {
	widgetTypeID, sceneID, err := parseJobKey(key)
	if err != nil {
		return err
	}

	var scenes []scene.Scene
	if sceneID != nil {
		sc, err := p.scenes.FindByID(ctx, *sceneID)
		if errors.Is(err, scene.ErrSceneNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot find scene %s: %w", *sceneID, err)
		}
		scenes = []scene.Scene{sc}
	} else if scenes, err = p.scenes.FindByWidgetTypeID(ctx, widgetTypeID); err != nil {
		return fmt.Errorf("cannot list scenes with widgets of type %s: %w", widgetTypeID, err)
	}

	if err := changeEachScene(ctx, p.scenes, scenes, p.reconcile(widgetTypeID)); err != nil {
		return fmt.Errorf("cannot reconcile with widget type %s: %w", widgetTypeID, err)
	}
	return nil
}

// parseJobKey returns the WidgetType of a job key and its Scene, nil for
// every Scene of the WidgetType.
func parseJobKey(key string) (id.ID[library.WidgetType], *id.ID[scene.Scene], error) {
	rest, ok := strings.CutPrefix(key, widgetTypeKeyPrefix)
	if !ok {
		return id.ID[library.WidgetType]{}, nil, fmt.Errorf("invalid job key %q", key)
	}
	rawTypeID, rawSceneID, oneScene := strings.Cut(rest, sceneKeySeparator)
	typeUUID, err := uuid.Parse(rawTypeID)
	if err != nil {
		return id.ID[library.WidgetType]{}, nil, fmt.Errorf("invalid widget type ID in job key %q: %w", key, err)
	}
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](typeUUID))
	if !oneScene {
		return widgetTypeID, nil, nil
	}
	sceneUUID, err := uuid.Parse(rawSceneID)
	if err != nil {
		return id.ID[library.WidgetType]{}, nil, fmt.Errorf("invalid scene ID in job key %q: %w", key, err)
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](sceneUUID))
	return widgetTypeID, &sceneID, nil
}

// reconcile returns the change reconciling a Scene with the WidgetType
// widgetTypeID and the Tags its Widgets of that type bind. The WidgetType and
// the Tags are read after the Scene, at every attempt of changeScene, so
// every Widget the Scene holds was checked against them or older versions,
// and no binding valid now is taken for an orphan. A deleted WidgetType
// leaves nothing to clean: its Widgets are gone with it.
func (p widgetReconciliationPolicy) reconcile(widgetTypeID id.ID[library.WidgetType]) sceneChange {
	return func(ctx context.Context, sc scene.Scene) (scene.Scene, bool, error) {
		wt, err := p.widgetTypes.FindByID(ctx, widgetTypeID)
		if errors.Is(err, library.ErrWidgetTypeNotFound) {
			return sc, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("cannot read widget type: %w", err)
		}
		tagTypes, err := loadTagTypes(ctx, p.tags, sc.TagsBoundByType(widgetTypeID))
		if err != nil {
			return nil, false, err
		}
		reconciled, changed := sc.ReconcileWith(wt, tagTypes)
		return reconciled, changed, nil
	}
}
