package event

import "github.com/kipitix/growscada/internal/domain/widget"

// WidgetTypeDeletedEvent - event published when a widget type is deleted.
type WidgetTypeDeletedEvent interface {
	WidgetTypeEvent
}

type widgetTypeDeletedEventImpl struct {
	widgetTypeEventImpl
}

var _ WidgetTypeDeletedEvent = (*widgetTypeDeletedEventImpl)(nil)

func NewWidgetTypeDeletedEvent(anID widget.WidgetTypeID, opts ...EventOption) WidgetTypeDeletedEvent {
	ev := &widgetTypeDeletedEventImpl{
		widgetTypeEventImpl: widgetTypeEventImpl{
			widgetTypeID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeWidgetTypeDeleted,
				timestamp: NewEventTimestamp(),
			},
		},
	}

	ev.applyOptions(opts...)

	return ev
}
