package library

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// WidgetTypeCreatedEvent is recorded by CreateWidgetType.
type WidgetTypeCreatedEvent interface {
	WidgetTypeEvent
}

type widgetTypeCreatedEventImpl struct {
	widgetTypeEventImpl
}

var _ WidgetTypeCreatedEvent = (*widgetTypeCreatedEventImpl)(nil)

func NewWidgetTypeCreatedEvent(anID id.ID[WidgetType], opts ...event.EventOption) WidgetTypeCreatedEvent {
	return &widgetTypeCreatedEventImpl{
		widgetTypeEventImpl: widgetTypeEventImpl{
			Base:         event.NewBase(event.EventTypeWidgetTypeCreated, opts...),
			widgetTypeID: anID,
		},
	}
}
