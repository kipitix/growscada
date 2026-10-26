package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// SceneEvent - base interface for the events a Scene records about itself.
// A WidgetEvent carries a SceneID too but is no SceneEvent: sceneEvent tells
// them apart, so a type switch need not order its cases. Both are
// project.ContentEvents: they name the Scene's Project.
type SceneEvent interface {
	project.ContentEvent
	SceneID() id.ID[Scene]
	sceneEvent()
}

type sceneEventImpl struct {
	event.Base
	sceneID id.ID[Scene]
	project.Content
}

var _ SceneEvent = (*sceneEventImpl)(nil)

// NO FABRIC METHOD

func (e sceneEventImpl) SceneID() id.ID[Scene] {
	return e.sceneID
}

func (sceneEventImpl) sceneEvent() {}
