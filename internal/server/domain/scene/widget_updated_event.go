package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// WidgetUpdatedEvent is recorded by Scene.UpdateWidget and, for each widget it changes, Scene.ReconcileWith.
type WidgetUpdatedEvent interface {
	WidgetEvent
}

type widgetUpdatedEventImpl struct {
	widgetEventImpl
}

var _ WidgetUpdatedEvent = (*widgetUpdatedEventImpl)(nil)

func NewWidgetUpdatedEvent(aWidgetID id.ID[Widget], aSceneID id.ID[Scene], opts ...event.EventOption) WidgetUpdatedEvent {
	return &widgetUpdatedEventImpl{
		widgetEventImpl: widgetEventImpl{
			Base:     event.NewBase(event.EventTypeWidgetUpdated, opts...),
			widgetID: aWidgetID,
			sceneID:  aSceneID,
		},
	}
}
