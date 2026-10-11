package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
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
	repository library.WidgetTypeRepository
}

var _ WidgetTypeService = (*widgetTypeServiceImpl)(nil)

// NewWidgetTypeService returns the widget type service. The events of its
// changes are recorded by the aggregates and saved with them (ADR 0008).
func NewWidgetTypeService(aRepository library.WidgetTypeRepository) WidgetTypeService {
	return &widgetTypeServiceImpl{
		repository: aRepository,
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

	def, err := parseWidgetTypeDefinition(input)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type: %w", invalidInput(err))
	}
	newWt, err := library.CreateWidgetType(newID, def.name, def.html, def.script, def.lang, def.defaultSize, def.inputPorts)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type: %w", invalidInput(err))
	}

	newWt, err = s.repository.Save(ctx, newWt)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}

	return appdto.NewWidgetType(newWt), nil
}

func (s widgetTypeServiceImpl) UpdateWidgetType(ctx context.Context, rawID uuid.UUID, expectedVersion int, input appdto.WidgetTypeInput) (appdto.WidgetType, error) {
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](rawID))

	found, err := s.repository.FindByID(ctx, widgetTypeID)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
	}

	// A negative version is no version a widget type has ever had: an edit conflict.
	expected, err := version.New(version.WithNumber[library.WidgetType](expectedVersion))
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("%w: %w", library.ErrWidgetTypeConflict, err)
	}
	// A stale write is a conflict whatever it sends: the version is checked
	// before the definition is parsed (Update checks it again, as its own rule).
	if err := found.CheckVersion(expected); err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", err)
	}

	def, err := parseWidgetTypeDefinition(input)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", invalidInput(err))
	}
	updated, err := found.Update(expected, def.name, def.html, def.script, def.lang, def.defaultSize, def.inputPorts)
	if errors.Is(err, library.ErrInvalidWidgetType) {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", invalidInput(err))
	}
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", err)
	}

	// The Widgets of the type are brought into line later, by the policy on
	// widget_type_updated (NewWidgetReconciliationPolicy, ADR 0008).
	found, err = s.repository.Save(ctx, updated)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}

	return appdto.NewWidgetType(found), nil
}

// widgetTypeDefinition is a widget type's definition as the client sent it,
// each field checked on its own; the WidgetType checks them together (e.g.
// port names are unique).
type widgetTypeDefinition struct {
	name        library.WidgetTypeName
	html        library.HtmlTemplate
	script      library.Script
	lang        library.ScriptLanguage
	defaultSize library.Size
	inputPorts  []library.InputPort
}

// parseWidgetTypeDefinition parses the client's fields; each error names the
// field it comes from. The caller marks the error as invalid input.
func parseWidgetTypeDefinition(input appdto.WidgetTypeInput) (widgetTypeDefinition, error) {
	var def widgetTypeDefinition
	var err error
	if def.name, err = library.NewWidgetTypeName(input.Name); err != nil {
		return def, fmt.Errorf("name: %w", err)
	}
	if def.html, err = library.NewHtmlTemplate(input.HtmlTemplate); err != nil {
		return def, fmt.Errorf("html_template: %w", err)
	}
	if def.script, err = library.NewScript(input.Script); err != nil {
		return def, fmt.Errorf("script: %w", err)
	}
	if def.lang, err = library.NewScriptLanguage(input.ScriptLanguage); err != nil {
		return def, fmt.Errorf("script_language: %w", err)
	}
	if def.defaultSize, err = library.NewSize(input.DefaultWidth, input.DefaultHeight); err != nil {
		return def, fmt.Errorf("default_width, default_height: %w", err)
	}
	if def.inputPorts, err = dtoInputPortsToDomain(input.InputPorts); err != nil {
		return def, fmt.Errorf("input_ports: %w", err)
	}
	return def, nil
}

// DeleteWidgetTypeByID deletes a widget type no Widget uses; while Widgets
// use it, the error wraps library.ErrWidgetTypeInUse. It expects no Version:
// a widget type changed meanwhile is reread and deleted as it is now
// (retryOnRace).
func (s widgetTypeServiceImpl) DeleteWidgetTypeByID(ctx context.Context, rawID uuid.UUID) (appdto.WidgetType, error) {
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](rawID))

	var deleted library.WidgetType
	err := retryOnRace(library.ErrWidgetTypeConflict, func() error {
		found, err := s.repository.FindByID(ctx, widgetTypeID)
		if err != nil {
			return fmt.Errorf("error on find widget type by id in repository: %w", err)
		}
		if err := s.repository.Delete(ctx, found.Delete()); err != nil {
			return fmt.Errorf("cannot delete widget type: %w", err)
		}
		deleted = found
		return nil
	})
	if err != nil {
		return appdto.WidgetType{}, err
	}

	return appdto.NewWidgetType(deleted), nil
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
