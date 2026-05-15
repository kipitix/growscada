package event

import "github.com/kipitix/growscada/internal/domain/widget"

// WidgetTypeCreatedEvent - event published when a widget type is created.
type WidgetTypeCreatedEvent interface {
	WidgetTypeEvent
}

type widgetTypeCreatedEventImpl struct {
	widgetTypeEventImpl
}

var _ WidgetTypeCreatedEvent = (*widgetTypeCreatedEventImpl)(nil)

func NewWidgetTypeCreatedEvent(anID widget.WidgetTypeID, opts ...EventOption) WidgetTypeCreatedEvent {
	ev := &widgetTypeCreatedEventImpl{
		widgetTypeEventImpl: widgetTypeEventImpl{
			widgetTypeID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeWidgetTypeCreated,
				timestamp: NewEventTimestamp(),
			},
		},
	}

	ev.applyOptions(opts...)

	return ev
}
