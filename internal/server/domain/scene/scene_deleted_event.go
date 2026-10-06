package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// SceneDeletedEvent is recorded by Scene.Delete.
type SceneDeletedEvent interface {
	SceneEvent
}

type sceneDeletedEventImpl struct {
	sceneEventImpl
}

var _ SceneDeletedEvent = (*sceneDeletedEventImpl)(nil)

func NewSceneDeletedEvent(anID id.ID[Scene], opts ...event.EventOption) SceneDeletedEvent {
	return &sceneDeletedEventImpl{
		sceneEventImpl: sceneEventImpl{
			Base:    event.NewBase(event.EventTypeSceneDeleted, opts...),
			sceneID: anID,
		},
	}
}
