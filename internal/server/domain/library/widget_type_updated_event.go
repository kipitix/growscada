package library

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// WidgetTypeUpdatedEvent is recorded by WidgetType.Update.
type WidgetTypeUpdatedEvent interface {
	WidgetTypeEvent
}

type widgetTypeUpdatedEventImpl struct {
	widgetTypeEventImpl
}

var _ WidgetTypeUpdatedEvent = (*widgetTypeUpdatedEventImpl)(nil)

func NewWidgetTypeUpdatedEvent(anID id.ID[WidgetType], opts ...event.EventOption) WidgetTypeUpdatedEvent {
	return &widgetTypeUpdatedEventImpl{
		widgetTypeEventImpl: widgetTypeEventImpl{
			Base:         event.NewBase(event.EventTypeWidgetTypeUpdated, opts...),
			widgetTypeID: anID,
		},
	}
}
