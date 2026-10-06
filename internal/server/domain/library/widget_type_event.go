package library

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// WidgetTypeEvent - base interface for all events a WidgetType records.
type WidgetTypeEvent interface {
	event.Event
	WidgetTypeID() id.ID[WidgetType]
}

type widgetTypeEventImpl struct {
	event.Base
	widgetTypeID id.ID[WidgetType]
}

var _ WidgetTypeEvent = (*widgetTypeEventImpl)(nil)

// NO FABRIC METHOD

func (e widgetTypeEventImpl) WidgetTypeID() id.ID[WidgetType] {
	return e.widgetTypeID
}
