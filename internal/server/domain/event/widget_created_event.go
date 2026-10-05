package event

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// WidgetCreatedEvent - event published when a widget instance is created.
type WidgetCreatedEvent interface {
	WidgetEvent
}

type widgetCreatedEventImpl struct {
	widgetEventImpl
}

var _ WidgetCreatedEvent = (*widgetCreatedEventImpl)(nil)

func NewWidgetCreatedEvent(anID id.ID[scene.Widget], opts ...EventOption) WidgetCreatedEvent {
	ev := &widgetCreatedEventImpl{
		widgetEventImpl: widgetEventImpl{
			widgetID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeWidgetCreated,
				timestamp: NewEventTimestamp(),
			},
		},
	}
	ev.applyOptions(opts...)
	return ev
}
