package event

import (
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// WidgetTypeEvent - base interface for all widget type domain events.
type WidgetTypeEvent interface {
	Event
	WidgetTypeID() id.ID[widget.WidgetType]
}

type widgetTypeEventImpl struct {
	eventImpl
	widgetTypeID id.ID[widget.WidgetType]
}

var _ WidgetTypeEvent = (*widgetTypeEventImpl)(nil)

// NO FABRIC METHOD

func (e widgetTypeEventImpl) WidgetTypeID() id.ID[widget.WidgetType] {
	return e.widgetTypeID
}
