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
	FindWidgetsBySceneID(context.Context, uuid.UUID) ([]appdto.Widget, error)
	FindWidgetByID(context.Context, uuid.UUID) (appdto.Widget, error)
	CreateWidget(context.Context, appdto.CreateWidgetInput) (appdto.Widget, error)
	UpdateWidget(context.Context, appdto.UpdateWidgetInput) (appdto.Widget, error)
	DeleteWidgetByID(context.Context, uuid.UUID) (appdto.Widget, error)
}

type widgetServiceImpl struct {
	repository         widget.WidgetRepository
	widgetTypeRepository widget.WidgetTypeRepository
	eventBus           event.EventBus
}

var _ WidgetService = (*widgetServiceImpl)(nil)

func NewWidgetService(repo widget.WidgetRepository, wtRepo widget.WidgetTypeRepository, bus event.EventBus) WidgetService {
	return &widgetServiceImpl{repository: repo, widgetTypeRepository: wtRepo, eventBus: bus}
}

func (s widgetServiceImpl) FindAllWidgets(ctx context.Context) ([]appdto.Widget, error) {
	list, err := s.repository.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find widgets in repository: %w", err)
	}
	return appdto.NewWidgetList(list), nil
}

func (s widgetServiceImpl) FindWidgetsBySceneID(ctx context.Context, rawSceneID uuid.UUID) ([]appdto.Widget, error) {
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	list, err := s.repository.FindBySceneID(ctx, sceneID)
	if err != nil {
		return nil, fmt.Errorf("error on find widgets by scene id in repository: %w", err)
	}
	return appdto.NewWidgetList(list), nil
}

func (s widgetServiceImpl) FindWidgetByID(ctx context.Context, rawID uuid.UUID) (appdto.Widget, error) {
	widgetID := id.NewID(id.IDWithUUID[widget.Widget](rawID))
	found, err := s.repository.FindByID(ctx, widgetID)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("error on find widget by id in repository: %w", err)
	}
	return appdto.NewWidget(found), nil
}

func (s widgetServiceImpl) CreateWidget(ctx context.Context, input appdto.CreateWidgetInput) (appdto.Widget, error) {
	if input.SceneID == uuid.Nil {
		return appdto.Widget{}, fmt.Errorf("scene_id is required: %w", widget.ErrWidgetInvalidInput)
	}
	if input.TypeID == uuid.Nil {
		return appdto.Widget{}, fmt.Errorf("type_id is required: %w", widget.ErrWidgetInvalidInput)
	}

	newID := s.repository.NextID()

	newName, err := widget.NewWidgetName(input.Name)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget because of name: %w", err)
	}

	pos := widget.NewPosition(input.X, input.Y, input.Z)

	size, err := widget.NewSize(input.Width, input.Height)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget because of size: %w", err)
	}

	origin, err := widget.NewOrigin(input.OriginX, input.OriginY)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget because of origin: %w", err)
	}

	rotation := widget.NewRotation(input.RotationDegrees)

	typeID := id.NewID(id.IDWithUUID[widget.WidgetType](input.TypeID))
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](input.SceneID))

	portBindings, err := dtoPortBindingsToDomain(input.PortBindings)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget because of port bindings: %w", err)
	}

	if len(portBindings) > 0 {
		if err := s.validatePortBindingsAgainstType(ctx, typeID, portBindings); err != nil {
			return appdto.Widget{}, fmt.Errorf("cannot create widget because of port bindings: %w", err)
		}
	}

	newWidget := widget.NewWidget(
		newID, newName, pos, size, origin, rotation,
		typeID, sceneID, input.Labels, portBindings,
		version.Initial[widget.Widget](),
	)

	saved, err := s.repository.Save(ctx, newWidget)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot save widget: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetCreatedEvent(newID))

	return appdto.NewWidget(saved), nil
}

func (s widgetServiceImpl) UpdateWidget(ctx context.Context, input appdto.UpdateWidgetInput) (appdto.Widget, error) {
	if input.SceneID == uuid.Nil {
		return appdto.Widget{}, fmt.Errorf("scene_id is required: %w", widget.ErrWidgetInvalidInput)
	}
	if input.TypeID == uuid.Nil {
		return appdto.Widget{}, fmt.Errorf("type_id is required: %w", widget.ErrWidgetInvalidInput)
	}

	widgetID := id.NewID(id.IDWithUUID[widget.Widget](input.ID))

	found, err := s.repository.FindByID(ctx, widgetID)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("error finding widget for update: %w", err)
	}

	if found.Version().Number() != input.Version {
		return appdto.Widget{}, widget.ErrWidgetConflict
	}

	newName, err := widget.NewWidgetName(input.Name)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot parse widget name: %w", err)
	}

	pos := widget.NewPosition(input.X, input.Y, input.Z)

	size, err := widget.NewSize(input.Width, input.Height)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot parse widget size: %w", err)
	}

	origin, err := widget.NewOrigin(input.OriginX, input.OriginY)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot parse widget origin: %w", err)
	}

	rotation := widget.NewRotation(input.RotationDegrees)

	typeID := id.NewID(id.IDWithUUID[widget.WidgetType](input.TypeID))
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](input.SceneID))

	portBindings, err := dtoPortBindingsToDomain(input.PortBindings)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot parse widget port bindings: %w", err)
	}

	if len(portBindings) > 0 {
		if err := s.validatePortBindingsAgainstType(ctx, typeID, portBindings); err != nil {
			return appdto.Widget{}, fmt.Errorf("cannot update widget because of port bindings: %w", err)
		}
	}

	updated := widget.NewWidget(
		found.ID(), newName, pos, size, origin, rotation,
		typeID, sceneID, input.Labels, portBindings,
		found.Version(),
	)

	saved, err := s.repository.Save(ctx, updated)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot save widget: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetUpdatedEvent(widgetID))

	return appdto.NewWidget(saved), nil
}

func (s widgetServiceImpl) DeleteWidgetByID(ctx context.Context, rawID uuid.UUID) (appdto.Widget, error) {
	widgetID := id.NewID(id.IDWithUUID[widget.Widget](rawID))

	deleted, err := s.repository.DeleteByID(ctx, widgetID)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot delete widget: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetDeletedEvent(widgetID))

	return appdto.NewWidget(deleted), nil
}

// validatePortBindingsAgainstType checks that every PortBinding port name is
// declared on the given WidgetType. Called only when bindings is non-empty.
func (s widgetServiceImpl) validatePortBindingsAgainstType(ctx context.Context, typeID id.ID[widget.WidgetType], bindings []widget.PortBinding) error {
	wt, err := s.widgetTypeRepository.FindByID(ctx, typeID)
	if err != nil {
		return fmt.Errorf("cannot find widget type for port binding validation: %w", err)
	}
	allowed := make(map[string]struct{}, len(wt.InputPorts()))
	for _, p := range wt.InputPorts() {
		allowed[p.Name().String()] = struct{}{}
	}
	for _, b := range bindings {
		if _, ok := allowed[b.PortName().String()]; !ok {
			return fmt.Errorf("port %q is not declared on widget type %q: %w",
				b.PortName().String(), wt.Name().String(), widget.ErrWidgetInvalidInput)
		}
	}
	return nil
}

// dtoPortBindingsToDomain converts appdto.PortBinding slice to domain PortBinding slice.
func dtoPortBindingsToDomain(dtos []appdto.PortBinding) ([]widget.PortBinding, error) {
	bindings := make([]widget.PortBinding, 0, len(dtos))
	for _, dto := range dtos {
		portName, err := widget.NewInputPortName(dto.PortName)
		if err != nil {
			return nil, fmt.Errorf("invalid port binding name %q: %w", dto.PortName, err)
		}
		tagID := id.NewID(id.IDWithUUID[tag.Tag](dto.TagID))
		bindings = append(bindings, widget.NewPortBinding(portName, tagID))
	}
	return bindings, nil
}
