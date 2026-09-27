package event

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// SceneUpdatedEvent - event published when a scene is updated.
type SceneUpdatedEvent interface {
	SceneEvent
}

type sceneUpdatedEventImpl struct {
	sceneEventImpl
}

var _ SceneUpdatedEvent = (*sceneUpdatedEventImpl)(nil)

func NewSceneUpdatedEvent(anID id.ID[scene.Scene], opts ...EventOption) SceneUpdatedEvent {
	ev := &sceneUpdatedEventImpl{
		sceneEventImpl: sceneEventImpl{
			sceneID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeSceneUpdated,
				timestamp: NewEventTimestamp(),
			},
		},
	}
	ev.applyOptions(opts...)
	return ev
}
