package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// WidgetTypeService is the service interface for working with widget types.
type WidgetTypeService interface {
	FindAllWidgetTypes(context.Context) ([]appdto.WidgetType, error)
	FindWidgetTypeByID(context.Context, uuid.UUID) (appdto.WidgetType, error)
	CreateWidgetType(context.Context, appdto.WidgetTypeInput) (appdto.WidgetType, error)
	UpdateWidgetType(ctx context.Context, widgetTypeID uuid.UUID, version int, input appdto.WidgetTypeInput) (appdto.WidgetType, error)
	DeleteWidgetTypeByID(context.Context, uuid.UUID) (appdto.WidgetType, error)
}

type widgetTypeServiceImpl struct {
	repository      library.WidgetTypeRepository
	sceneRepository scene.SceneRepository
	eventBus        event.EventBus
}

var _ WidgetTypeService = (*widgetTypeServiceImpl)(nil)

func NewWidgetTypeService(aRepository library.WidgetTypeRepository, aSceneRepository scene.SceneRepository, anEventBus event.EventBus) WidgetTypeService {
	return &widgetTypeServiceImpl{
		repository:      aRepository,
		sceneRepository: aSceneRepository,
		eventBus:        anEventBus,
	}
}

func (s widgetTypeServiceImpl) FindAllWidgetTypes(ctx context.Context) ([]appdto.WidgetType, error) {
	list, err := s.repository.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find widget types in repository: %w", err)
	}
	return appdto.NewWidgetTypeList(list), nil
}

func (s widgetTypeServiceImpl) FindWidgetTypeByID(ctx context.Context, rawID uuid.UUID) (appdto.WidgetType, error) {
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](rawID))
	wt, err := s.repository.FindByID(ctx, widgetTypeID)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
	}
	return appdto.NewWidgetType(wt), nil
}

func (s widgetTypeServiceImpl) CreateWidgetType(ctx context.Context, input appdto.WidgetTypeInput) (appdto.WidgetType, error) {
	newID := s.repository.NextID()

	newWt, err := parseWidgetType(newID, version.Initial[library.WidgetType](), input)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type: %w", invalidInput(err))
	}

	newWt, err = s.repository.Save(ctx, newWt)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetTypeCreatedEvent(newID))

	return appdto.NewWidgetType(newWt), nil
}

func (s widgetTypeServiceImpl) UpdateWidgetType(ctx context.Context, rawID uuid.UUID, expectedVersion int, input appdto.WidgetTypeInput) (appdto.WidgetType, error) {
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](rawID))

	found, err := s.repository.FindByID(ctx, widgetTypeID)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
	}

	if found.Version().Number() != expectedVersion {
		return appdto.WidgetType{}, library.ErrWidgetTypeConflict
	}

	updated, err := parseWidgetType(found.ID(), found.Version(), input)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", invalidInput(err))
	}

	found, err = s.repository.Save(ctx, updated)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetTypeUpdatedEvent(widgetTypeID))

	if err := s.removeOrphanedPortBindings(ctx, found); err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot clean up orphaned port bindings: %w", err)
	}

	return appdto.NewWidgetType(found), nil
}

// parseWidgetType builds a widget type from the client's fields; each error
// names the field it comes from. The caller marks the error as invalid input.
func parseWidgetType(widgetTypeID id.ID[library.WidgetType], ver version.Version[library.WidgetType], input appdto.WidgetTypeInput) (library.WidgetType, error) {
	name, err := library.NewWidgetTypeName(input.Name)
	if err != nil {
		return nil, fmt.Errorf("name: %w", err)
	}
	html, err := library.NewHtmlTemplate(input.HtmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("html_template: %w", err)
	}
	script, err := library.NewScript(input.Script)
	if err != nil {
		return nil, fmt.Errorf("script: %w", err)
	}
	lang, err := library.NewScriptLanguage(input.ScriptLanguage)
	if err != nil {
		return nil, fmt.Errorf("script_language: %w", err)
	}
	defaultSize, err := library.NewSize(input.DefaultWidth, input.DefaultHeight)
	if err != nil {
		return nil, fmt.Errorf("default_width, default_height: %w", err)
	}
	inputPorts, err := dtoInputPortsToDomain(input.InputPorts)
	if err != nil {
		return nil, fmt.Errorf("input_ports: %w", err)
	}
	// NewWidgetType checks the fields together (e.g. port names are unique).
	return library.NewWidgetType(widgetTypeID, name, html, script, lang, defaultSize, inputPorts, ver)
}

// DeleteWidgetTypeByID deletes a widget type no Widget uses; while Widgets
// use it, the error wraps library.ErrWidgetTypeInUse.
func (s widgetTypeServiceImpl) DeleteWidgetTypeByID(ctx context.Context, rawID uuid.UUID) (appdto.WidgetType, error) {
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](rawID))

	deleted, err := s.repository.DeleteByID(ctx, widgetTypeID)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot delete widget type: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetTypeDeletedEvent(widgetTypeID))

	return appdto.NewWidgetType(deleted), nil
}

// removeOrphanedPortBindings removes, from every Widget of WidgetType wt,
// the PortBindings of ports wt no longer declares: each Scene holding such
// Widgets reconciles them with wt and is saved if anything changed. The
// Scenes are saved one by one, not atomically (task 21).
func (s widgetTypeServiceImpl) removeOrphanedPortBindings(ctx context.Context, wt library.WidgetType) error {
	scenes, err := s.sceneRepository.FindByWidgetTypeID(ctx, wt.ID())
	if err != nil {
		return fmt.Errorf("cannot list scenes with widgets of type %s: %w", wt.ID(), err)
	}

	for _, sc := range scenes {
		if err := s.reconcileScene(ctx, sc, wt); err != nil {
			return fmt.Errorf("cannot save scene %s after port binding cleanup: %w", sc.ID(), err)
		}
	}
	return nil
}

// maxReconcileAttempts bounds how often reconcileScene rereads a Scene that
// keeps changing under it.
const maxReconcileAttempts = 3

// reconcileScene reconciles sc with wt and saves it if anything changed.
// The change does not rest on what a client saw, so a Scene changed since it
// was read is reread and reconciled again rather than reported as a
// conflict; a Scene deleted meanwhile has nothing left to clean.
func (s widgetTypeServiceImpl) reconcileScene(ctx context.Context, sc scene.Scene, wt library.WidgetType) error {
	for attempt := 1; ; attempt++ {
		reconciled, changed := sc.ReconcileWith(wt)
		if !changed {
			return nil
		}
		_, err := s.sceneRepository.Save(ctx, reconciled)
		if err == nil || errors.Is(err, scene.ErrSceneNotFound) {
			return nil
		}
		if !errors.Is(err, scene.ErrSceneConflict) || attempt == maxReconcileAttempts {
			return err
		}
		sc, err = s.sceneRepository.FindByID(ctx, sc.ID())
		if errors.Is(err, scene.ErrSceneNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot reread scene: %w", err)
		}
	}
}

// dtoInputPortsToDomain converts appdto.InputPort slice to domain InputPort slice.
func dtoInputPortsToDomain(dtos []appdto.InputPort) ([]library.InputPort, error) {
	ports := make([]library.InputPort, 0, len(dtos))
	for _, dto := range dtos {
		name, err := library.NewInputPortName(dto.Name)
		if err != nil {
			return nil, fmt.Errorf("invalid input port name %q: %w", dto.Name, err)
		}
		typeHint, err := library.NewPortTypeHint(dto.TypeHint)
		if err != nil {
			return nil, fmt.Errorf("invalid input port type hint %q: %w", dto.TypeHint, err)
		}
		ports = append(ports, library.NewInputPort(name, dto.Description, typeHint))
	}
	return ports, nil
}
