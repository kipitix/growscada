package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// WidgetUpdatedEvent is recorded by Scene.UpdateWidget and, for each widget it changes, Scene.ReconcileWith.
type WidgetUpdatedEvent interface {
	WidgetTypedEvent
}

type widgetUpdatedEventImpl struct {
	widgetTypedEventImpl
}

var _ WidgetUpdatedEvent = (*widgetUpdatedEventImpl)(nil)

func NewWidgetUpdatedEvent(aWidgetID id.ID[Widget], aSceneID id.ID[Scene], aWidgetTypeID id.ID[library.WidgetType], opts ...event.EventOption) WidgetUpdatedEvent {
	return newWidgetUpdatedEvent(aWidgetID, aSceneID, &aWidgetTypeID, opts...)
}

// NewWidgetUpdatedEventOfUnknownType rebuilds a WidgetUpdatedEvent stored
// before events carried the WidgetType. For the outbox codec.
func NewWidgetUpdatedEventOfUnknownType(aWidgetID id.ID[Widget], aSceneID id.ID[Scene], opts ...event.EventOption) WidgetUpdatedEvent {
	return newWidgetUpdatedEvent(aWidgetID, aSceneID, nil, opts...)
}

func newWidgetUpdatedEvent(aWidgetID id.ID[Widget], aSceneID id.ID[Scene], aWidgetTypeID *id.ID[library.WidgetType], opts ...event.EventOption) WidgetUpdatedEvent {
	return &widgetUpdatedEventImpl{
		widgetTypedEventImpl: widgetTypedEventImpl{
			widgetEventImpl: widgetEventImpl{
				Base:     event.NewBase(event.EventTypeWidgetUpdated, opts...),
				widgetID: aWidgetID,
				sceneID:  aSceneID,
			},
			widgetTypeID: aWidgetTypeID,
		},
	}
}
