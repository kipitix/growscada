package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// WidgetCreatedEvent is recorded by Scene.AddWidget.
type WidgetCreatedEvent interface {
	WidgetEvent
}

type widgetCreatedEventImpl struct {
	widgetEventImpl
}

var _ WidgetCreatedEvent = (*widgetCreatedEventImpl)(nil)

func NewWidgetCreatedEvent(aWidgetID id.ID[Widget], aSceneID id.ID[Scene], opts ...event.EventOption) WidgetCreatedEvent {
	return &widgetCreatedEventImpl{
		widgetEventImpl: widgetEventImpl{
			Base:     event.NewBase(event.EventTypeWidgetCreated, opts...),
			widgetID: aWidgetID,
			sceneID:  aSceneID,
		},
	}
}
