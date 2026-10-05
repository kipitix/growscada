package event

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// WidgetEvent - base interface for all widget instance domain events.
type WidgetEvent interface {
	Event
	WidgetID() id.ID[scene.Widget]
}

type widgetEventImpl struct {
	eventImpl
	widgetID id.ID[scene.Widget]
}

var _ WidgetEvent = (*widgetEventImpl)(nil)

// NO FABRIC METHOD

func (e widgetEventImpl) WidgetID() id.ID[scene.Widget] {
	return e.widgetID
}
