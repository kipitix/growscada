package application

import (
	"context"
	"fmt"

	"gitverse.ru/kipitix/growscada/internal/application/dto"
	"gitverse.ru/kipitix/growscada/internal/domain/tag"
)

// TagService - интерфейс сервиса для работы с тегами
type TagService interface {
	FindAllTags(context.Context) (dto.TagList, error)
}

// tagServiceImpl - структура реализации сервиса тегов
type tagServiceImpl struct {
	tagRepository tag.TagRepository
}

var _ TagService = (*tagServiceImpl)(nil)

// NewTagService создает и возвращает новый экземпляр сервиса тегов
// Возвращает реализацию интерфейса TagService
func NewTagService(aTagRepository tag.TagRepository) TagService {
	return &tagServiceImpl{
		tagRepository: aTagRepository,
	}
}

// FindAllTags возвращает список всех тегов
func (t tagServiceImpl) FindAllTags(ctx context.Context) (dto.TagList, error) {
	tags, err := t.tagRepository.FindAll(ctx)
	if err != nil {
		return dto.TagList{}, fmt.Errorf("error on find tags in repository: %w", err)
	}

	return dto.NewTagList(tags), nil
}
