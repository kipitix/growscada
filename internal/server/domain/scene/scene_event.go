package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// SceneEvent - base interface for the events a Scene records about itself.
type SceneEvent interface {
	event.Event
	SceneID() id.ID[Scene]
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
