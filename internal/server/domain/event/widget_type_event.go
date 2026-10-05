package event

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// WidgetTypeEvent - base interface for all widget type domain events.
type WidgetTypeEvent interface {
	Event
	WidgetTypeID() id.ID[library.WidgetType]
}

type widgetTypeEventImpl struct {
	eventImpl
	widgetTypeID id.ID[library.WidgetType]
}

var _ WidgetTypeEvent = (*widgetTypeEventImpl)(nil)

// NO FABRIC METHOD

func (e widgetTypeEventImpl) WidgetTypeID() id.ID[library.WidgetType] {
	return e.widgetTypeID
}
