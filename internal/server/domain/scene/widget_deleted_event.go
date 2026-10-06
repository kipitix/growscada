package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// WidgetDeletedEvent is recorded by Scene.RemoveWidget and, for each widget, Scene.Delete.
type WidgetDeletedEvent interface {
	WidgetEvent
}

type widgetDeletedEventImpl struct {
	widgetEventImpl
}

var _ WidgetDeletedEvent = (*widgetDeletedEventImpl)(nil)

func NewWidgetDeletedEvent(aWidgetID id.ID[Widget], aSceneID id.ID[Scene], opts ...event.EventOption) WidgetDeletedEvent {
	return &widgetDeletedEventImpl{
		widgetEventImpl: widgetEventImpl{
			Base:     event.NewBase(event.EventTypeWidgetDeleted, opts...),
			widgetID: aWidgetID,
			sceneID:  aSceneID,
		},
	}
}
