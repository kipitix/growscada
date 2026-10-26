package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// SceneUpdatedEvent is recorded by Scene.Update.
type SceneUpdatedEvent interface {
	SceneEvent
}

type sceneUpdatedEventImpl struct {
	sceneEventImpl
}

var _ SceneUpdatedEvent = (*sceneUpdatedEventImpl)(nil)

func NewSceneUpdatedEvent(anID id.ID[Scene], aProjectID id.ID[project.Project], opts ...event.EventOption) SceneUpdatedEvent {
	return &sceneUpdatedEventImpl{
		sceneEventImpl: sceneEventImpl{
			Base:    event.NewBase(event.EventTypeSceneUpdated, opts...),
			sceneID: anID,
			Content: project.NewContent(aProjectID),
		},
	}
}
