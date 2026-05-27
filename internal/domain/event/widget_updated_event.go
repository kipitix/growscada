package event

import (
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// WidgetUpdatedEvent - event published when a widget instance is updated.
type WidgetUpdatedEvent interface {
	WidgetEvent
}

type widgetUpdatedEventImpl struct {
	widgetEventImpl
}

var _ WidgetUpdatedEvent = (*widgetUpdatedEventImpl)(nil)

func NewWidgetUpdatedEvent(anID id.ID[widget.Widget], opts ...EventOption) WidgetUpdatedEvent {
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
