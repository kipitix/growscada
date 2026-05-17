package event

import (
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
)

// SceneDeletedEvent - event published when a scene is deleted.
type SceneDeletedEvent interface {
	SceneEvent
}

type sceneDeletedEventImpl struct {
	sceneEventImpl
}

var _ SceneDeletedEvent = (*sceneDeletedEventImpl)(nil)

func NewSceneDeletedEvent(anID id.ID[scene.Scene], opts ...EventOption) SceneDeletedEvent {
	ev := &sceneDeletedEventImpl{
		sceneEventImpl: sceneEventImpl{
			sceneID: anID,
			eventImpl: eventImpl{
				eventType: EventTypeSceneDeleted,
				timestamp: NewEventTimestamp(),
			},
		},
	}
	ev.applyOptions(opts...)
	return ev
}
