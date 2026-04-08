package application

import (
	"context"

	"gitverse.ru/kipitix/growscada/internal/application/dto"
)

// TagService - интерфейс сервиса для работы с тегами
type TagService interface {
	TagList(context.Context) (dto.TagList, error)
}

// tagServiceImpl - структура реализации сервиса тегов
type tagServiceImpl struct {
}

var _ TagService = (*tagServiceImpl)(nil)

// NewTagService создает и возвращает новый экземпляр сервиса тегов
// Возвращает реализацию интерфейса TagService
func NewTagService() TagService {
	return &tagServiceImpl{}
}

func (t tagServiceImpl) TagList(ctx context.Context) (dto.TagList, error) {
	return dto.TagList{}, nil
}
