package library

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// WidgetTypeDeletedEvent is recorded by WidgetType.Delete.
type WidgetTypeDeletedEvent interface {
	WidgetTypeEvent
}

type widgetTypeDeletedEventImpl struct {
	widgetTypeEventImpl
}

var _ WidgetTypeDeletedEvent = (*widgetTypeDeletedEventImpl)(nil)

func NewWidgetTypeDeletedEvent(anID id.ID[WidgetType], opts ...event.EventOption) WidgetTypeDeletedEvent {
	return &widgetTypeDeletedEventImpl{
		widgetTypeEventImpl: widgetTypeEventImpl{
			Base:         event.NewBase(event.EventTypeWidgetTypeDeleted, opts...),
			widgetTypeID: anID,
		},
	}
}
