package application

import (
	"context"
	"fmt"

	"github.com/kipitix/growscada/internal/application/app_dto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/indicator_type"
)

// IndicatorTypeService is the service interface for working with indicator types.
type IndicatorTypeService interface {
	FindAllIndicatorTypes(context.Context) ([]app_dto.IndicatorType, error)
	FindIndicatorTypeByID(context.Context, indicator_type.IndicatorTypeID) (app_dto.IndicatorType, error)
	CreateIndicatorType(context.Context, app_dto.CreateIndicatorTypeInput) (app_dto.IndicatorType, error)
	UpdateIndicatorType(context.Context, app_dto.UpdateIndicatorTypeInput) (app_dto.IndicatorType, error)
	DeleteIndicatorTypeByID(context.Context, indicator_type.IndicatorTypeID) (app_dto.IndicatorType, error)
}

type indicatorTypeServiceImpl struct {
	repository indicator_type.IndicatorTypeRepository
	eventBus   event.EventBus
}

var _ IndicatorTypeService = (*indicatorTypeServiceImpl)(nil)

func NewIndicatorTypeService(aRepository indicator_type.IndicatorTypeRepository, anEventBus event.EventBus) IndicatorTypeService {
	return &indicatorTypeServiceImpl{
		repository: aRepository,
		eventBus:   anEventBus,
	}
}

func (s indicatorTypeServiceImpl) FindAllIndicatorTypes(ctx context.Context) ([]app_dto.IndicatorType, error) {
	list, err := s.repository.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find indicator types in repository: %w", err)
	}
	return app_dto.NewIndicatorTypeList(list), nil
}

func (s indicatorTypeServiceImpl) FindIndicatorTypeByID(ctx context.Context, id indicator_type.IndicatorTypeID) (app_dto.IndicatorType, error) {
	it, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("error on find indicator type by id in repository: %w", err)
	}
	return app_dto.NewIndicatorType(it), nil
}

func (s indicatorTypeServiceImpl) CreateIndicatorType(ctx context.Context, input app_dto.CreateIndicatorTypeInput) (app_dto.IndicatorType, error) {
	newID := s.repository.NextID()

	newName, err := indicator_type.NewIndicatorTypeName(input.Name)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot create indicator type because of name: %w", err)
	}

	newSvg, err := indicator_type.NewSvgTemplate(input.SvgTemplate)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot create indicator type because of svg template: %w", err)
	}

	newScript, err := indicator_type.NewScript(input.Script)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot create indicator type because of script: %w", err)
	}

	newLang, err := indicator_type.NewScriptLanguage(input.ScriptLanguage)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot create indicator type because of script language: %w", err)
	}

	newIt, err := indicator_type.NewIndicatorType(newID, newName, newSvg, newScript, newLang, indicator_type.IndicatorTypeVersionInitial)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot create indicator type: %w", err)
	}

	if err = s.repository.Save(ctx, newIt); err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot save indicator type: %w", err)
	}

	s.eventBus.Publish(event.NewIndicatorTypeCreatedEvent(newID))

	return app_dto.NewIndicatorType(newIt), nil
}

func (s indicatorTypeServiceImpl) UpdateIndicatorType(ctx context.Context, input app_dto.UpdateIndicatorTypeInput) (app_dto.IndicatorType, error) {
	id := indicator_type.NewIndicatorTypeID(indicator_type.IndicatorTypeIDWithUUID(input.ID))

	found, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("error on find indicator type by id in repository: %w", err)
	}

	newName, err := indicator_type.NewIndicatorTypeName(input.Name)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot parse name: %w", err)
	}

	newSvg, err := indicator_type.NewSvgTemplate(input.SvgTemplate)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot parse svg template: %w", err)
	}

	newScript, err := indicator_type.NewScript(input.Script)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot parse script: %w", err)
	}

	newLang, err := indicator_type.NewScriptLanguage(input.ScriptLanguage)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot parse script language: %w", err)
	}

	found.Update(newName, newSvg, newScript, newLang)

	if err = s.repository.Save(ctx, found); err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot save indicator type: %w", err)
	}

	s.eventBus.Publish(event.NewIndicatorTypeUpdatedEvent(id))

	return app_dto.NewIndicatorType(found), nil
}

func (s indicatorTypeServiceImpl) DeleteIndicatorTypeByID(ctx context.Context, id indicator_type.IndicatorTypeID) (app_dto.IndicatorType, error) {
	deleted, err := s.repository.DeleteByID(ctx, id)
	if err != nil {
		return app_dto.IndicatorType{}, fmt.Errorf("cannot delete indicator type: %w", err)
	}

	s.eventBus.Publish(event.NewIndicatorTypeDeletedEvent(id))

	return app_dto.NewIndicatorType(deleted), nil
}
