package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/version"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// WidgetTypeService is the service interface for working with widget types.
type WidgetTypeService interface {
	FindAllWidgetTypes(context.Context) ([]appdto.WidgetType, error)
	FindWidgetTypeByID(context.Context, uuid.UUID) (appdto.WidgetType, error)
	CreateWidgetType(context.Context, appdto.CreateWidgetTypeInput) (appdto.WidgetType, error)
	UpdateWidgetType(context.Context, appdto.UpdateWidgetTypeInput) (appdto.WidgetType, error)
	DeleteWidgetTypeByID(context.Context, uuid.UUID) (appdto.WidgetType, error)
}

type widgetTypeServiceImpl struct {
	repository       widget.WidgetTypeRepository
	widgetRepository widget.WidgetRepository
	eventBus         event.EventBus
}

var _ WidgetTypeService = (*widgetTypeServiceImpl)(nil)

func NewWidgetTypeService(aRepository widget.WidgetTypeRepository, aWidgetRepository widget.WidgetRepository, anEventBus event.EventBus) WidgetTypeService {
	return &widgetTypeServiceImpl{
		repository:       aRepository,
		widgetRepository: aWidgetRepository,
		eventBus:         anEventBus,
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
	widgetTypeID := id.NewID(id.IDWithUUID[widget.WidgetType](rawID))
	wt, err := s.repository.FindByID(ctx, widgetTypeID)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
	}
	return appdto.NewWidgetType(wt), nil
}

func (s widgetTypeServiceImpl) CreateWidgetType(ctx context.Context, input appdto.CreateWidgetTypeInput) (appdto.WidgetType, error) {
	newID := s.repository.NextID()

	newName, err := widget.NewWidgetTypeName(input.Name)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type because of name: %w", err)
	}

	newHtml, err := widget.NewHtmlTemplate(input.HtmlTemplate)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type because of html template: %w", err)
	}

	newScript, err := widget.NewScript(input.Script)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type because of script: %w", err)
	}

	newLang, err := widget.NewScriptLanguage(input.ScriptLanguage)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type because of script language: %w", err)
	}

	defaultSize, err := widget.NewSize(input.DefaultWidth, input.DefaultHeight)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type because of default size: %w", err)
	}

	inputPorts, err := dtoInputPortsToDomain(input.InputPorts)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type because of input ports: %w", err)
	}

	newWt, err := widget.NewWidgetType(newID, newName, newHtml, newScript, newLang, defaultSize, inputPorts, version.Initial[widget.WidgetType]())
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type: %w", err)
	}

	newWt, err = s.repository.Save(ctx, newWt)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetTypeCreatedEvent(newID))

	return appdto.NewWidgetType(newWt), nil
}

func (s widgetTypeServiceImpl) UpdateWidgetType(ctx context.Context, input appdto.UpdateWidgetTypeInput) (appdto.WidgetType, error) {
	widgetTypeID := id.NewID(id.IDWithUUID[widget.WidgetType](input.ID))

	found, err := s.repository.FindByID(ctx, widgetTypeID)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
	}

	if found.Version().Number() != input.Version {
		return appdto.WidgetType{}, widget.ErrWidgetTypeConflict
	}

	newName, err := widget.NewWidgetTypeName(input.Name)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot parse name: %w", err)
	}

	newHtml, err := widget.NewHtmlTemplate(input.HtmlTemplate)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot parse html template: %w", err)
	}

	newScript, err := widget.NewScript(input.Script)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot parse script: %w", err)
	}

	newLang, err := widget.NewScriptLanguage(input.ScriptLanguage)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot parse script language: %w", err)
	}

	defaultSize, err := widget.NewSize(input.DefaultWidth, input.DefaultHeight)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot parse default size: %w", err)
	}

	inputPorts, err := dtoInputPortsToDomain(input.InputPorts)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot parse input ports: %w", err)
	}

	updated, err := widget.NewWidgetType(found.ID(), newName, newHtml, newScript, newLang, defaultSize, inputPorts, found.Version())
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot build updated widget type: %w", err)
	}

	found, err = s.repository.Save(ctx, updated)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetTypeUpdatedEvent(widgetTypeID))

	if err := s.removeOrphanedPortBindings(ctx, widgetTypeID, updated.InputPorts()); err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot clean up orphaned port bindings: %w", err)
	}

	return appdto.NewWidgetType(found), nil
}

func (s widgetTypeServiceImpl) DeleteWidgetTypeByID(ctx context.Context, rawID uuid.UUID) (appdto.WidgetType, error) {
	widgetTypeID := id.NewID(id.IDWithUUID[widget.WidgetType](rawID))

	deleted, err := s.repository.DeleteByID(ctx, widgetTypeID)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot delete widget type: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetTypeDeletedEvent(widgetTypeID))

	// Clear all port bindings on widgets that referenced the deleted type.
	if err := s.removeOrphanedPortBindings(ctx, widgetTypeID, nil); err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot clean up orphaned port bindings after delete: %w", err)
	}

	return appdto.NewWidgetType(deleted), nil
}

// removeOrphanedPortBindings finds all widgets using the given widget type and
// removes any PortBindings whose port name is not in allowedPorts. Passing nil
// (or an empty slice) removes all port bindings — used on type deletion.
func (s widgetTypeServiceImpl) removeOrphanedPortBindings(ctx context.Context, typeID id.ID[widget.WidgetType], allowedPorts []widget.InputPort) error {
	allowed := make(map[string]struct{}, len(allowedPorts))
	for _, p := range allowedPorts {
		allowed[p.Name().String()] = struct{}{}
	}

	widgets, err := s.widgetRepository.FindByTypeID(ctx, typeID)
	if err != nil {
		return fmt.Errorf("cannot list widgets for type %s: %w", typeID, err)
	}

	for _, w := range widgets {
		filtered := make([]widget.PortBinding, 0, len(w.PortBindings()))
		for _, b := range w.PortBindings() {
			if _, ok := allowed[b.PortName().String()]; ok {
				filtered = append(filtered, b)
			}
		}
		if len(filtered) == len(w.PortBindings()) {
			continue // nothing to remove
		}
		updated := widget.NewWidget(
			w.ID(), w.Name(), w.Position(), w.Size(), w.Origin(), w.Rotation(),
			w.TypeID(), w.SceneID(), w.Labels(), filtered, w.Version(),
		)
		if _, err := s.widgetRepository.Save(ctx, updated); err != nil {
			return fmt.Errorf("cannot save widget %s after port binding cleanup: %w", w.ID(), err)
		}
	}
	return nil
}

// dtoInputPortsToDomain converts appdto.InputPort slice to domain InputPort slice.
func dtoInputPortsToDomain(dtos []appdto.InputPort) ([]widget.InputPort, error) {
	ports := make([]widget.InputPort, 0, len(dtos))
	for _, dto := range dtos {
		name, err := widget.NewInputPortName(dto.Name)
		if err != nil {
			return nil, fmt.Errorf("invalid input port name %q: %w", dto.Name, err)
		}
		typeHint, err := tag.NewTagType(dto.TypeHint)
		if err != nil {
			return nil, fmt.Errorf("invalid input port type hint %q: %w", dto.TypeHint, err)
		}
		ports = append(ports, widget.NewInputPort(name, dto.Description, typeHint))
	}
	return ports, nil
}
