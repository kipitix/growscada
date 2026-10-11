package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// WidgetCreatedEvent is recorded by Scene.AddWidget.
type WidgetCreatedEvent interface {
	WidgetTypedEvent
}

type widgetCreatedEventImpl struct {
	widgetTypedEventImpl
}

var _ WidgetCreatedEvent = (*widgetCreatedEventImpl)(nil)

func NewWidgetCreatedEvent(aWidgetID id.ID[Widget], aSceneID id.ID[Scene], aWidgetTypeID id.ID[library.WidgetType], opts ...event.EventOption) WidgetCreatedEvent {
	return &widgetCreatedEventImpl{
		widgetTypedEventImpl: widgetTypedEventImpl{
			widgetEventImpl: widgetEventImpl{
				Base:     event.NewBase(event.EventTypeWidgetCreated, opts...),
				widgetID: aWidgetID,
				sceneID:  aSceneID,
			},
			widgetTypeID: aWidgetTypeID,
		},
	}
}
