package event

import (
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// WidgetEvent - base interface for all widget instance domain events.
type WidgetEvent interface {
	Event
	WidgetID() id.ID[widget.Widget]
}

type widgetEventImpl struct {
	eventImpl
	widgetID id.ID[widget.Widget]
}

var _ WidgetEvent = (*widgetEventImpl)(nil)

// NO FABRIC METHOD

func (e widgetEventImpl) WidgetID() id.ID[widget.Widget] {
	return e.widgetID
}
