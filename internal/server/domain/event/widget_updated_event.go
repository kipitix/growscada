package event

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// WidgetUpdatedEvent - event published when a widget instance is updated.
type WidgetUpdatedEvent interface {
	WidgetEvent
}

type widgetUpdatedEventImpl struct {
	widgetEventImpl
}

var _ WidgetUpdatedEvent = (*widgetUpdatedEventImpl)(nil)

func NewWidgetUpdatedEvent(anID id.ID[scene.Widget], opts ...EventOption) WidgetUpdatedEvent {
	ev := &widgetUpdatedEventImpl{
		widgetEventImpl: widgetEventImpl{
			widgetID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeWidgetUpdated,
				timestamp: NewEventTimestamp(),
			},
		},
	}
	ev.applyOptions(opts...)
	return ev
}
