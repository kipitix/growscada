package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// WidgetEvent - base interface for the events a Scene records about one of
// its widgets.
type WidgetEvent interface {
	event.Event
	WidgetID() id.ID[Widget]
	// SceneID is the Scene that holds (or held) the widget.
	SceneID() id.ID[Scene]
}

type widgetEventImpl struct {
	event.Base
	widgetID id.ID[Widget]
	sceneID  id.ID[Scene]
}

var _ WidgetEvent = (*widgetEventImpl)(nil)

// NO FABRIC METHOD

func (e widgetEventImpl) WidgetID() id.ID[Widget] {
	return e.widgetID
}

func (e widgetEventImpl) SceneID() id.ID[Scene] {
	return e.sceneID
}
