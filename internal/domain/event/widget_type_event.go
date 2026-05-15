package event

import "github.com/kipitix/growscada/internal/domain/widget"

// WidgetTypeEvent - base interface for all widget type domain events.
type WidgetTypeEvent interface {
	Event
	WidgetTypeID() widget.WidgetTypeID
}

type widgetTypeEventImpl struct {
	eventImpl
	widgetTypeID widget.WidgetTypeID
}

var _ WidgetTypeEvent = (*widgetTypeEventImpl)(nil)

// NO FABRIC METHOD

func (e widgetTypeEventImpl) WidgetTypeID() widget.WidgetTypeID {
	return e.widgetTypeID
}
