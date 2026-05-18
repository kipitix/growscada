package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/scene"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/version"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// WidgetService is the service interface for working with widget instances.
type WidgetService interface {
	FindAllWidgets(context.Context) ([]appdto.Widget, error)
	FindWidgetByID(context.Context, id.ID[widget.Widget]) (appdto.Widget, error)
	CreateWidget(context.Context, appdto.CreateWidgetInput) (appdto.Widget, error)
	UpdateWidget(context.Context, appdto.UpdateWidgetInput) (appdto.Widget, error)
	DeleteWidgetByID(context.Context, id.ID[widget.Widget]) (appdto.Widget, error)
}

type widgetServiceImpl struct {
	repository widget.WidgetRepository
	eventBus   event.EventBus
}

var _ WidgetService = (*widgetServiceImpl)(nil)

func NewWidgetService(repo widget.WidgetRepository, bus event.EventBus) WidgetService {
	return &widgetServiceImpl{repository: repo, eventBus: bus}
}

func (s widgetServiceImpl) FindAllWidgets(ctx context.Context) ([]appdto.Widget, error) {
	list, err := s.repository.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find widgets in repository: %w", err)
	}
	return appdto.NewWidgetList(list), nil
}

func (s widgetServiceImpl) FindWidgetByID(ctx context.Context, widgetID id.ID[widget.Widget]) (appdto.Widget, error) {
	found, err := s.repository.FindByID(ctx, widgetID)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("error on find widget by id in repository: %w", err)
	}
	return appdto.NewWidget(found), nil
}

func (s widgetServiceImpl) CreateWidget(ctx context.Context, input appdto.CreateWidgetInput) (appdto.Widget, error) {
	newID := s.repository.NextID()

	newName, err := widget.NewWidgetName(input.Name)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget because of name: %w", err)
	}

	coords := widget.NewCoordinates(input.X, input.Y, input.Z)

	size, err := normalizeSize(input.Width, input.Height)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget because of size: %w", err)
	}

	typeID, err := id.NewID(id.IDWithUUID[widget.WidgetType](input.TypeID))
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget because of type id: %w", err)
	}

	sceneID, err := id.NewID(id.IDWithUUID[scene.Scene](input.SceneID))
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget because of scene id: %w", err)
	}

	tagIDs, err := uuidsToTagIDs(input.TagIDs)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget because of tag ids: %w", err)
	}

	newWidget, err := widget.NewWidget(newID, newName, coords, size, typeID, sceneID, input.Labels, tagIDs, version.Initial[widget.Widget]())
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget: %w", err)
	}

	saved, err := s.repository.Save(ctx, newWidget)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot save widget: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetCreatedEvent(newID))

	return appdto.NewWidget(saved), nil
}

func (s widgetServiceImpl) UpdateWidget(ctx context.Context, input appdto.UpdateWidgetInput) (appdto.Widget, error) {
	widgetID, err := id.NewID(id.IDWithUUID[widget.Widget](input.ID))
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot build widget id: %w", err)
	}

	found, err := s.repository.FindByID(ctx, widgetID)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("error on find widget by id in repository: %w", err)
	}

	newName, err := widget.NewWidgetName(input.Name)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot parse widget name: %w", err)
	}

	coords := widget.NewCoordinates(input.X, input.Y, input.Z)

	size, err := normalizeSize(input.Width, input.Height)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot parse widget size: %w", err)
	}

	typeID, err := id.NewID(id.IDWithUUID[widget.WidgetType](input.TypeID))
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot parse type id: %w", err)
	}

	sceneID, err := id.NewID(id.IDWithUUID[scene.Scene](input.SceneID))
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot parse scene id: %w", err)
	}

	tagIDs, err := uuidsToTagIDs(input.TagIDs)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot parse tag ids: %w", err)
	}

	updated, err := widget.NewWidget(found.ID(), newName, coords, size, typeID, sceneID, input.Labels, tagIDs, found.Version())
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot build updated widget: %w", err)
	}

	saved, err := s.repository.Save(ctx, updated)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot save widget: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetUpdatedEvent(widgetID))

	return appdto.NewWidget(saved), nil
}

func (s widgetServiceImpl) DeleteWidgetByID(ctx context.Context, widgetID id.ID[widget.Widget]) (appdto.Widget, error) {
	deleted, err := s.repository.DeleteByID(ctx, widgetID)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot delete widget: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetDeletedEvent(widgetID))

	return appdto.NewWidget(deleted), nil
}

func uuidsToTagIDs(uuids []uuid.UUID) ([]id.ID[tag.Tag], error) {
	tagIDs := make([]id.ID[tag.Tag], len(uuids))
	for i, u := range uuids {
		tid, err := id.NewID(id.IDWithUUID[tag.Tag](u))
		if err != nil {
			return nil, err
		}
		tagIDs[i] = tid
	}
	return tagIDs, nil
}
