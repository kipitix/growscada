package event

import (
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// WidgetCreatedEvent - event published when a widget instance is created.
type WidgetCreatedEvent interface {
	WidgetEvent
}

type widgetCreatedEventImpl struct {
	widgetEventImpl
}

var _ WidgetCreatedEvent = (*widgetCreatedEventImpl)(nil)

func NewWidgetCreatedEvent(anID id.ID[widget.Widget], opts ...EventOption) WidgetCreatedEvent {
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
