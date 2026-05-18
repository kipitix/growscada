package application

import (
	"context"
	"fmt"

	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/version"
	"github.com/kipitix/growscada/internal/domain/widget"
)

// WidgetTypeService is the service interface for working with widget types.
type WidgetTypeService interface {
	FindAllWidgetTypes(context.Context) ([]appdto.WidgetType, error)
	FindWidgetTypeByID(context.Context, widget.WidgetTypeID) (appdto.WidgetType, error)
	CreateWidgetType(context.Context, appdto.CreateWidgetTypeInput) (appdto.WidgetType, error)
	UpdateWidgetType(context.Context, appdto.UpdateWidgetTypeInput) (appdto.WidgetType, error)
	DeleteWidgetTypeByID(context.Context, widget.WidgetTypeID) (appdto.WidgetType, error)
}

type widgetTypeServiceImpl struct {
	repository widget.WidgetTypeRepository
	eventBus   event.EventBus
}

var _ WidgetTypeService = (*widgetTypeServiceImpl)(nil)

func NewWidgetTypeService(aRepository widget.WidgetTypeRepository, anEventBus event.EventBus) WidgetTypeService {
	return &widgetTypeServiceImpl{
		repository: aRepository,
		eventBus:   anEventBus,
	}
}

func (s widgetTypeServiceImpl) FindAllWidgetTypes(ctx context.Context) ([]appdto.WidgetType, error) {
	list, err := s.repository.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find widget types in repository: %w", err)
	}
	return appdto.NewWidgetTypeList(list), nil
}

func (s widgetTypeServiceImpl) FindWidgetTypeByID(ctx context.Context, id widget.WidgetTypeID) (appdto.WidgetType, error) {
	wt, err := s.repository.FindByID(ctx, id)
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

	defaultSize, err := normalizeSize(input.DefaultWidth, input.DefaultHeight)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type because of default size: %w", err)
	}

	newWt := widget.NewWidgetType(newID, newName, newHtml, newScript, newLang, defaultSize, version.Initial[widget.WidgetType]())

	newWt, err = s.repository.Save(ctx, newWt)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetTypeCreatedEvent(newID))

	return appdto.NewWidgetType(newWt), nil
}

func (s widgetTypeServiceImpl) UpdateWidgetType(ctx context.Context, input appdto.UpdateWidgetTypeInput) (appdto.WidgetType, error) {
	id := widget.NewWidgetTypeID(widget.WidgetTypeIDWithUUID(input.ID))

	found, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
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

	defaultSize, err := normalizeSize(input.DefaultWidth, input.DefaultHeight)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot parse default size: %w", err)
	}

	updated := widget.NewWidgetType(found.ID(), newName, newHtml, newScript, newLang, defaultSize, found.Version())

	found, err = s.repository.Save(ctx, updated)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetTypeUpdatedEvent(id))

	return appdto.NewWidgetType(found), nil
}

func (s widgetTypeServiceImpl) DeleteWidgetTypeByID(ctx context.Context, id widget.WidgetTypeID) (appdto.WidgetType, error) {
	deleted, err := s.repository.DeleteByID(ctx, id)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot delete widget type: %w", err)
	}

	s.eventBus.Publish(event.NewWidgetTypeDeletedEvent(id))

	return appdto.NewWidgetType(deleted), nil
}

// normalizeSize defaults zero dimensions to 100 and calls NewSize.
func normalizeSize(width, height int) (widget.Size, error) {
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 100
	}
	return widget.NewSize(width, height)
}
