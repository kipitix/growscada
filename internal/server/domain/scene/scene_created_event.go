package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// SceneCreatedEvent is recorded by CreateScene.
type SceneCreatedEvent interface {
	SceneEvent
}

type sceneCreatedEventImpl struct {
	sceneEventImpl
}

var _ SceneCreatedEvent = (*sceneCreatedEventImpl)(nil)

func NewSceneCreatedEvent(anID id.ID[Scene], aProjectID id.ID[project.Project], opts ...event.EventOption) SceneCreatedEvent {
	return &sceneCreatedEventImpl{
		sceneEventImpl: sceneEventImpl{
			Base:    event.NewBase(event.EventTypeSceneCreated, opts...),
			sceneID: anID,
			Content: project.NewContent(aProjectID),
		},
	}
}
