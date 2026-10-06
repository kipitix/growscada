package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
)

// SceneCreatedEvent is recorded by CreateScene.
type SceneCreatedEvent interface {
	SceneEvent
}

type sceneCreatedEventImpl struct {
	sceneEventImpl
}

var _ SceneCreatedEvent = (*sceneCreatedEventImpl)(nil)

func NewSceneCreatedEvent(anID id.ID[Scene], opts ...event.EventOption) SceneCreatedEvent {
	return &sceneCreatedEventImpl{
		sceneEventImpl: sceneEventImpl{
			Base:    event.NewBase(event.EventTypeSceneCreated, opts...),
			sceneID: anID,
		},
	}
}
