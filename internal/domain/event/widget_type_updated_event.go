package event

import "github.com/kipitix/growscada/internal/domain/widget"

// WidgetTypeUpdatedEvent - event published when a widget type is updated.
type WidgetTypeUpdatedEvent interface {
	WidgetTypeEvent
}

type widgetTypeUpdatedEventImpl struct {
	widgetTypeEventImpl
}

var _ WidgetTypeUpdatedEvent = (*widgetTypeUpdatedEventImpl)(nil)

func NewWidgetTypeUpdatedEvent(anID widget.WidgetTypeID, opts ...EventOption) WidgetTypeUpdatedEvent {
	ev := &widgetTypeUpdatedEventImpl{
		widgetTypeEventImpl: widgetTypeEventImpl{
			widgetTypeID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeWidgetTypeUpdated,
				timestamp: NewEventTimestamp(),
			},
		},
	}

	ev.applyOptions(opts...)

	return ev
}
