package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// WidgetReconciliationPolicyName is the Name of the policy returned by
// NewWidgetReconciliationPolicy.
const WidgetReconciliationPolicyName = "reconcile_widgets_with_widget_type"

// widgetReconciliationPolicy removes, from every Widget of a WidgetType, the
// PortBindings of ports the WidgetType no longer declares. Its job is keyed
// by the WidgetType: a widget_type_updated event may have dropped a port, and
// a widget_created or widget_updated event may have bound a port that a
// concurrent update of the WidgetType had just dropped (the Widget was
// checked against the WidgetType as read before that update).
//
// It is transitional: once a Project sees only a Library Release (ADR 0007),
// reconciliation moves to the DraftChange of a pinned release (task 33).
type widgetReconciliationPolicy struct {
	widgetTypes library.WidgetTypeRepository
	scenes      scene.SceneRepository
}

var _ Policy = (*widgetReconciliationPolicy)(nil)

// NewWidgetReconciliationPolicy returns the policy bringing Widgets into line
// with their WidgetType.
func NewWidgetReconciliationPolicy(aWidgetTypeRepository library.WidgetTypeRepository, aSceneRepository scene.SceneRepository) Policy {
	return &widgetReconciliationPolicy{
		widgetTypes: aWidgetTypeRepository,
		scenes:      aSceneRepository,
	}
}

func (p widgetReconciliationPolicy) Name() string {
	return WidgetReconciliationPolicyName
}

func (p widgetReconciliationPolicy) JobKey(e event.Event) (string, bool) {
	switch e.Type() {
	case event.EventTypeWidgetTypeUpdated:
		if ev, ok := e.(library.WidgetTypeEvent); ok {
			return ev.WidgetTypeID().String(), true
		}
	case event.EventTypeWidgetCreated, event.EventTypeWidgetUpdated:
		if ev, ok := e.(scene.WidgetTypedEvent); ok {
			if typeID, ok := ev.WidgetTypeID(); ok {
				return typeID.String(), true
			}
		}
	}
	return "", false
}

// Run reconciles every Scene holding Widgets of the WidgetType key with the
// WidgetType as it is now, each Scene in its own transaction. A Scene that
// fails does not stop the others: Run returns the errors of all that failed,
// and the job is run again, the Scenes already reconciled then left as they
// are. A deleted WidgetType has no Widgets left to reconcile.
func (p widgetReconciliationPolicy) Run(ctx context.Context, key string) error {
	rawID, err := uuid.Parse(key)
	if err != nil {
		return fmt.Errorf("invalid widget type ID %q: %w", key, err)
	}
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](rawID))

	wt, err := p.widgetTypes.FindByID(ctx, widgetTypeID)
	if errors.Is(err, library.ErrWidgetTypeNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot find widget type %s: %w", widgetTypeID, err)
	}

	scenes, err := p.scenes.FindByWidgetTypeID(ctx, widgetTypeID)
	if err != nil {
		return fmt.Errorf("cannot list scenes with widgets of type %s: %w", widgetTypeID, err)
	}
	var errs []error
	for _, sc := range scenes {
		if err := p.reconcileScene(ctx, sc, wt); err != nil {
			errs = append(errs, fmt.Errorf("cannot reconcile scene %s with widget type %s: %w", sc.ID(), widgetTypeID, err))
		}
	}
	return errors.Join(errs...)
}

// reconcileScene reconciles the Scene sc with wt and saves it if anything
// changed. The change does not rest on what a client saw, so a Scene changed
// since it was read is reread and reconciled again rather than reported as a
// conflict (retryOnRace); a Scene deleted meanwhile has nothing left to clean.
func (p widgetReconciliationPolicy) reconcileScene(ctx context.Context, sc scene.Scene, wt library.WidgetType) error {
	first := true
	err := retryOnRace(scene.ErrSceneConflict, func() error {
		if !first {
			var err error
			if sc, err = p.scenes.FindByID(ctx, sc.ID()); err != nil {
				return fmt.Errorf("cannot reread scene: %w", err)
			}
		}
		first = false
		reconciled, changed := sc.ReconcileWith(wt)
		if !changed {
			return nil
		}
		_, err := p.scenes.Save(ctx, reconciled)
		return err
	})
	if errors.Is(err, scene.ErrSceneNotFound) {
		return nil
	}
	return err
}
