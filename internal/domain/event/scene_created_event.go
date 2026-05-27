package event

import (
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
)

// SceneCreatedEvent - event published when a scene is created.
type SceneCreatedEvent interface {
	SceneEvent
}

type sceneCreatedEventImpl struct {
	sceneEventImpl
}

var _ SceneCreatedEvent = (*sceneCreatedEventImpl)(nil)

func NewSceneCreatedEvent(anID id.ID[scene.Scene], opts ...EventOption) SceneCreatedEvent {
	ev := &sceneCreatedEventImpl{
		sceneEventImpl: sceneEventImpl{
			sceneID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeSceneCreated,
				timestamp: NewEventTimestamp(),
			},
		},
	}
	ev.applyOptions(opts...)
	return ev
}
