package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/domain/version"
)

// SceneService is the service interface for working with scenes.
type SceneService interface {
	FindAllScenes(context.Context) ([]appdto.Scene, error)
	FindSceneByID(context.Context, uuid.UUID) (appdto.Scene, error)
	CreateScene(context.Context, appdto.CreateSceneInput) (appdto.Scene, error)
	UpdateScene(context.Context, appdto.UpdateSceneInput) (appdto.Scene, error)
	DeleteSceneByID(context.Context, uuid.UUID) (appdto.Scene, error)
}

type sceneServiceImpl struct {
	repository scene.SceneRepository
	eventBus   event.EventBus
}

var _ SceneService = (*sceneServiceImpl)(nil)

func NewSceneService(repo scene.SceneRepository, bus event.EventBus) SceneService {
	return &sceneServiceImpl{repository: repo, eventBus: bus}
}

func (s sceneServiceImpl) FindAllScenes(ctx context.Context) ([]appdto.Scene, error) {
	list, err := s.repository.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find scenes in repository: %w", err)
	}
	return appdto.NewSceneList(list), nil
}

func (s sceneServiceImpl) FindSceneByID(ctx context.Context, rawID uuid.UUID) (appdto.Scene, error) {
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawID))
	found, err := s.repository.FindByID(ctx, sceneID)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("error on find scene by id in repository: %w", err)
	}
	return appdto.NewScene(found), nil
}

func (s sceneServiceImpl) CreateScene(ctx context.Context, input appdto.CreateSceneInput) (appdto.Scene, error) {
	newID := s.repository.NextID()

	newName, err := scene.NewSceneName(input.Name)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot create scene because of name: %w", err)
	}

	newSize, err := scene.NewSceneSize(input.Width, input.Height)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot create scene because of size: %w", err)
	}

	newScene := scene.NewScene(newID, newName, newSize, scene.NewBackgroundHTML(input.BackgroundHTML), version.Initial[scene.Scene]())

	saved, err := s.repository.Save(ctx, newScene)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot save scene: %w", err)
	}

	s.eventBus.Publish(event.NewSceneCreatedEvent(newID))

	return appdto.NewScene(saved), nil
}

func (s sceneServiceImpl) UpdateScene(ctx context.Context, input appdto.UpdateSceneInput) (appdto.Scene, error) {
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](input.ID))

	found, err := s.repository.FindByID(ctx, sceneID)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("error on find scene by id in repository: %w", err)
	}

	newName, err := scene.NewSceneName(input.Name)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot parse scene name: %w", err)
	}

	newSize, err := scene.NewSceneSize(input.Width, input.Height)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot parse scene size: %w", err)
	}

	updated := scene.NewScene(found.ID(), newName, newSize, scene.NewBackgroundHTML(input.BackgroundHTML), found.Version())

	saved, err := s.repository.Save(ctx, updated)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot save scene: %w", err)
	}

	s.eventBus.Publish(event.NewSceneUpdatedEvent(sceneID))

	return appdto.NewScene(saved), nil
}

func (s sceneServiceImpl) DeleteSceneByID(ctx context.Context, rawID uuid.UUID) (appdto.Scene, error) {
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawID))

	deleted, err := s.repository.DeleteByID(ctx, sceneID)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot delete scene: %w", err)
	}

	s.eventBus.Publish(event.NewSceneDeletedEvent(sceneID))

	return appdto.NewScene(deleted), nil
}
