package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// SceneEvent - base interface for the events a Scene records about itself.
// A WidgetEvent carries a SceneID too but is no SceneEvent: sceneEvent tells
// them apart, so a type switch need not order its cases.
type SceneEvent interface {
	event.Event
	SceneID() id.ID[Scene]
	sceneEvent()
}

type sceneEventImpl struct {
	event.Base
	sceneID id.ID[Scene]
}

var _ SceneEvent = (*sceneEventImpl)(nil)

// NO FABRIC METHOD

func (e sceneEventImpl) SceneID() id.ID[Scene] {
	return e.sceneID
}

func (sceneEventImpl) sceneEvent() {}
