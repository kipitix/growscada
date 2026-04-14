package dto

import (
	"github.com/google/uuid"
	"gitverse.ru/kipitix/growscada/internal/domain/tag"
)

// Tag - структура данных для передачи информации о теге через API
// Содержит основные атрибуты тега в формате, удобном для JSON-сериализации
type Tag struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Kind    string    `json:"kind"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
	Version int       `json:"version"`
}

// FindTagByIDResponse - структура для ответа с одним тегом
type FindTagByIDResponse struct {
	Tag Tag `json:"tag"`
}

// FindAllTagsResponse - структура для передачи списка тегов
// Используется для возврата коллекции тегов в API-ответах
type FindAllTagsResponse struct {
	Tags []Tag `json:"tags"`
}

// CreateTagRequest - структура для запроса на создание тега
type CreateTagRequest struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Value   string `json:"value"`
	Quality string `json:"quality"`
}

// CreateTagResponse - структура для ответа на создание тега
type CreateTagResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateTagRequest - структура для запроса на обновление тега
type UpdateTagRequest struct {
	ID      uuid.UUID `json:"id"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
}

// UpdateTagResponse - структура для ответа на обновление тега
type UpdateTagResponse struct {
	Version int `json:"version"`
}

// DeleteTagRequest - структура для запроса на удаление тега
type DeleteTagRequest struct {
	ID uuid.UUID `json:"id"`
}

// DeleteTagResponse - структура для ответа на удаление тега
type DeleteTagResponse struct {
	Tag Tag `json:"tag"`
}

// NewTag создает DTO-объект Tag на основе доменного агрегата tag.Tag
func NewTag(aTag tag.Tag) Tag {
	return Tag{
		ID:      aTag.ID().UUID(),
		Name:    aTag.Name().String(),
		Kind:    aTag.Kind().String(),
		Value:   aTag.Value().String(),
		Quality: aTag.Quality().String(),
		Version: aTag.Version(),
	}
}

// NewFindAllTagsResponse создает DTO-объект TagList на основе списка доменных агрегатов tag.Tag
func NewFindAllTagsResponse(aTagList []tag.Tag) FindAllTagsResponse {
	tags := make([]Tag, len(aTagList))
	for i, t := range aTagList {
		tags[i] = NewTag(t)
	}
	return FindAllTagsResponse{
		Tags: tags,
	}
}
