package application

import (
	"context"
	"errors"
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

// SceneService is the service interface for working with scenes and their
// widgets. Widget, as an entity of the Scene aggregate, has no service of its
// own: every widget operation is reached through a scene.
type SceneService interface {
	FindAllScenes(context.Context) ([]appdto.Scene, error)
	FindSceneByID(context.Context, uuid.UUID) (appdto.Scene, error)
	CreateScene(context.Context, appdto.CreateSceneInput) (appdto.Scene, error)
	UpdateScene(context.Context, appdto.UpdateSceneInput) (appdto.Scene, error)
	DeleteSceneByID(context.Context, uuid.UUID) (appdto.Scene, error)

	FindWidgetsBySceneID(ctx context.Context, sceneID uuid.UUID) ([]appdto.Widget, error)
	FindWidgetByID(ctx context.Context, sceneID, widgetID uuid.UUID) (appdto.Widget, error)
	CreateWidget(ctx context.Context, sceneID uuid.UUID, input appdto.CreateWidgetInput) (appdto.Widget, error)
	UpdateWidget(ctx context.Context, sceneID, widgetID uuid.UUID, input appdto.UpdateWidgetInput) (appdto.Widget, error)
	DeleteWidgetByID(ctx context.Context, sceneID, widgetID uuid.UUID) (appdto.Widget, error)
}

type sceneServiceImpl struct {
	repository           scene.SceneRepository
	widgetTypeRepository widget.WidgetTypeRepository
	eventBus             event.EventBus
}

var _ SceneService = (*sceneServiceImpl)(nil)

func NewSceneService(repo scene.SceneRepository, widgetTypeRepo widget.WidgetTypeRepository, bus event.EventBus) SceneService {
	return &sceneServiceImpl{repository: repo, widgetTypeRepository: widgetTypeRepo, eventBus: bus}
}

// validatePortBindingsAgainstType checks that every PortBinding port name is
// declared on the given WidgetType. Called only when bindings is non-empty.
func (s sceneServiceImpl) validatePortBindingsAgainstType(ctx context.Context, typeID id.ID[widget.WidgetType], bindings []widget.PortBinding) error {
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
		return appdto.Scene{}, fmt.Errorf("cannot create scene because of name: %w", errors.Join(scene.ErrSceneValidation, err))
	}

	newSize, err := scene.NewSceneSize(input.Width, input.Height)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot create scene because of size: %w", errors.Join(scene.ErrSceneValidation, err))
	}

	newScene := scene.NewScene(newID, newName, newSize, scene.NewBackgroundHTML(input.BackgroundHTML), nil, version.Initial[scene.Scene]())

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

	if found.Version().Number() != input.Version {
		return appdto.Scene{}, scene.ErrSceneConflict
	}

	newName, err := scene.NewSceneName(input.Name)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot parse scene name: %w", errors.Join(scene.ErrSceneValidation, err))
	}

	newSize, err := scene.NewSceneSize(input.Width, input.Height)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot parse scene size: %w", errors.Join(scene.ErrSceneValidation, err))
	}

	updated := scene.NewScene(found.ID(), newName, newSize, scene.NewBackgroundHTML(input.BackgroundHTML), found.Widgets(), found.Version())

	saved, err := s.repository.Save(ctx, updated)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot save scene: %w", err)
	}

	s.eventBus.Publish(event.NewSceneUpdatedEvent(sceneID))

	return appdto.NewScene(saved), nil
}

// DeleteSceneByID deletes a scene together with all of its widgets (the
// widgets are removed atomically in the DB via ON DELETE CASCADE). The
// widgets that existed immediately before deletion are read back from the
// domain result so that a WidgetDeletedEvent can be published for each one —
// keeping the event/audit trail honest about what the cascade actually did.
func (s sceneServiceImpl) DeleteSceneByID(ctx context.Context, rawID uuid.UUID) (appdto.Scene, error) {
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawID))

	deleted, err := s.repository.DeleteByID(ctx, sceneID)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot delete scene: %w", err)
	}

	for _, w := range deleted.Widgets() {
		s.eventBus.Publish(event.NewWidgetDeletedEvent(w.ID()))
	}
	s.eventBus.Publish(event.NewSceneDeletedEvent(sceneID))

	return appdto.NewScene(deleted), nil
}

func (s sceneServiceImpl) FindWidgetsBySceneID(ctx context.Context, rawSceneID uuid.UUID) ([]appdto.Widget, error) {
	sc, err := s.repository.FindByID(ctx, id.NewID(id.IDWithUUID[scene.Scene](rawSceneID)))
	if err != nil {
		return nil, fmt.Errorf("error on find scene by id in repository: %w", err)
	}
	return appdto.NewWidgetList(sc.Widgets(), rawSceneID, sc.Version().Number()), nil
}

func (s sceneServiceImpl) FindWidgetByID(ctx context.Context, rawSceneID, rawWidgetID uuid.UUID) (appdto.Widget, error) {
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	widgetID := id.NewID(id.IDWithUUID[widget.Widget](rawWidgetID))

	sc, err := s.repository.FindByID(ctx, sceneID)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("error on find scene by id in repository: %w", err)
	}

	// Scan the widgets already loaded with the scene instead of issuing a
	// second, separate query: two independent reads could otherwise observe
	// the scene at different points in time (e.g. a concurrent update lands
	// between them), pairing a stale SceneVersion with newer widget content
	// or vice versa.
	for _, w := range sc.Widgets() {
		if w.ID() == widgetID {
			return appdto.NewWidget(w, rawSceneID, sc.Version().Number()), nil
		}
	}
	return appdto.Widget{}, fmt.Errorf("error on find widget by id in repository: %w", widget.ErrWidgetNotFound)
}

func (s sceneServiceImpl) CreateWidget(ctx context.Context, rawSceneID uuid.UUID, input appdto.CreateWidgetInput) (appdto.Widget, error) {
	if input.TypeID == uuid.Nil {
		return appdto.Widget{}, fmt.Errorf("type_id is required: %w", widget.ErrWidgetInvalidInput)
	}

	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	newID := s.repository.NextWidgetID()

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
		typeID, input.Labels, portBindings,
	)

	expectedVersion, err := version.New(version.WithNumber[scene.Scene](input.SceneVersion))
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("invalid scene version: %w", err)
	}

	saved, newSceneVersion, err := s.repository.AddWidget(ctx, sceneID, expectedVersion, newWidget)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot save widget: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetCreatedEvent(newID))

	return appdto.NewWidget(saved, rawSceneID, newSceneVersion.Number()), nil
}

func (s sceneServiceImpl) UpdateWidget(ctx context.Context, rawSceneID, rawWidgetID uuid.UUID, input appdto.UpdateWidgetInput) (appdto.Widget, error) {
	if input.TypeID == uuid.Nil {
		return appdto.Widget{}, fmt.Errorf("type_id is required: %w", widget.ErrWidgetInvalidInput)
	}

	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	widgetID := id.NewID(id.IDWithUUID[widget.Widget](rawWidgetID))

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
		widgetID, newName, pos, size, origin, rotation,
		typeID, input.Labels, portBindings,
	)

	expectedVersion, err := version.New(version.WithNumber[scene.Scene](input.SceneVersion))
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("invalid scene version: %w", err)
	}

	saved, newSceneVersion, err := s.repository.UpdateWidget(ctx, sceneID, expectedVersion, updated)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot save widget: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetUpdatedEvent(widgetID))

	return appdto.NewWidget(saved, rawSceneID, newSceneVersion.Number()), nil
}

func (s sceneServiceImpl) DeleteWidgetByID(ctx context.Context, rawSceneID, rawWidgetID uuid.UUID) (appdto.Widget, error) {
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	widgetID := id.NewID(id.IDWithUUID[widget.Widget](rawWidgetID))

	deleted, newSceneVersion, err := s.repository.DeleteWidget(ctx, sceneID, widgetID)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot delete widget: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetDeletedEvent(widgetID))

	return appdto.NewWidget(deleted, rawSceneID, newSceneVersion.Number()), nil
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
