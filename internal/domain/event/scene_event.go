package event

import (
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
)

// SceneEvent - base interface for all scene domain events.
type SceneEvent interface {
	Event
	SceneID() id.ID[scene.Scene]
}

type sceneEventImpl struct {
	eventImpl
	sceneID id.ID[scene.Scene]
}

var _ SceneEvent = (*sceneEventImpl)(nil)

// NO FABRIC METHOD

func (e sceneEventImpl) SceneID() id.ID[scene.Scene] {
	return e.sceneID
}
