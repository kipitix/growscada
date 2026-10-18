package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// WidgetUpdatedEvent is recorded by Scene.UpdateWidget and, for each widget
// they change, Scene.ReconcileWith and Scene.UnbindTag.
type WidgetUpdatedEvent interface {
	WidgetTypedEvent
	// ByReconciliation reports that Scene.ReconcileWith or Scene.UnbindTag
	// recorded the event: the change only removed PortBindings that stopped
	// being valid, so it bound the widget to nothing new.
	ByReconciliation() bool
}

type widgetUpdatedEventImpl struct {
	widgetTypedEventImpl
	byReconciliation bool
}

var _ WidgetUpdatedEvent = (*widgetUpdatedEventImpl)(nil)

func NewWidgetUpdatedEvent(aWidgetID id.ID[Widget], aSceneID id.ID[Scene], aWidgetTypeID id.ID[library.WidgetType], opts ...event.EventOption) WidgetUpdatedEvent {
	return newWidgetUpdatedEvent(aWidgetID, aSceneID, aWidgetTypeID, false, opts...)
}

// NewWidgetUpdatedByReconciliationEvent returns the WidgetUpdatedEvent that
// Scene.ReconcileWith and Scene.UnbindTag record (ByReconciliation).
func NewWidgetUpdatedByReconciliationEvent(aWidgetID id.ID[Widget], aSceneID id.ID[Scene], aWidgetTypeID id.ID[library.WidgetType], opts ...event.EventOption) WidgetUpdatedEvent {
	return newWidgetUpdatedEvent(aWidgetID, aSceneID, aWidgetTypeID, true, opts...)
}

func newWidgetUpdatedEvent(aWidgetID id.ID[Widget], aSceneID id.ID[Scene], aWidgetTypeID id.ID[library.WidgetType], byReconciliation bool, opts ...event.EventOption) WidgetUpdatedEvent {
	return &widgetUpdatedEventImpl{
		widgetTypedEventImpl: widgetTypedEventImpl{
			widgetEventImpl: widgetEventImpl{
				Base:     event.NewBase(event.EventTypeWidgetUpdated, opts...),
				widgetID: aWidgetID,
				sceneID:  aSceneID,
			},
			widgetTypeID: aWidgetTypeID,
		},
		byReconciliation: byReconciliation,
	}
}

func (e widgetUpdatedEventImpl) ByReconciliation() bool {
	return e.byReconciliation
}
