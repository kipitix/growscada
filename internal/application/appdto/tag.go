package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/tag"
)

// Tag - DTO уровня прикладного сервиса для передачи данных тега.
// Поля не имеют json-тегов — сериализация выполняется на уровне интерфейса.
type Tag struct {
	ID      uuid.UUID
	Name    string
	Type    string
	Value   string
	Quality string
	Version int
}

// TagList - DTO уровня прикладного сервиса для передачи списка тегов.
type TagList struct {
	Tags []Tag
}

// CreateTagInput - входные данные для создания тега
type CreateTagInput struct {
	Name    string
	Type    string
	Value   string
	Quality string
}

// UpdateTagInput - входные данные для обновления значения тега
type UpdateTagInput struct {
	ID      uuid.UUID
	Value   string
	Quality string
}

// NewTag creates a Tag DTO from the tag.Tag domain aggregate
func NewTag(aTag tag.Tag) Tag {
	return Tag{
		ID:      aTag.ID().UUID(),
		Name:    aTag.Name().String(),
		Type:    aTag.Type().String(),
		Value:   aTag.Value().String(),
		Quality: aTag.Quality().String(),
		Version: aTag.Version(),
	}
}

// NewTagList creates a TagList DTO from a list of tag.Tag domain aggregates
func NewTagList(aTagList []tag.Tag) TagList {
	tags := make([]Tag, len(aTagList))
	for i, t := range aTagList {
		tags[i] = NewTag(t)
	}
	return TagList{Tags: tags}
}
