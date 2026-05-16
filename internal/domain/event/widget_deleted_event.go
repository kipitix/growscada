package event

import (
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// WidgetDeletedEvent - event published when a widget instance is deleted.
type WidgetDeletedEvent interface {
	WidgetEvent
}

type widgetDeletedEventImpl struct {
	widgetEventImpl
}

var _ WidgetDeletedEvent = (*widgetDeletedEventImpl)(nil)

func NewWidgetDeletedEvent(anID id.ID[widget.Widget], opts ...EventOption) WidgetDeletedEvent {
	ev := &widgetDeletedEventImpl{
		widgetEventImpl: widgetEventImpl{
			widgetID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeWidgetDeleted,
				timestamp: NewEventTimestamp(),
			},
		},
	}
	ev.applyOptions(opts...)
	return ev
}
