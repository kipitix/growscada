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

// TagList - структура для передачи списка тегов
// Используется для возврата коллекции тегов в API-ответах
type TagList struct {
	Tags []Tag `json:"tags"`
}

// NewTag создает DTO-объект Tag на основе доменного агрегата tag.Tag
func NewTag(aTag tag.Tag) Tag {
	return Tag{
		ID:      aTag.ID().UUID(),
		Name:    aTag.Name(),
		Kind:    aTag.Kind().String(),
		Value:   aTag.Value().String(),
		Quality: aTag.Quality().String(),
		Version: aTag.Version(),
	}
}

// NewTagList создает DTO-объект TagList на основе списка доменных агрегатов tag.Tag
func NewTagList(aTagList []tag.Tag) TagList {
	tags := make([]Tag, len(aTagList))
	for i, t := range aTagList {
		tags[i] = NewTag(t)
	}
	return TagList{
		Tags: tags,
	}
}
