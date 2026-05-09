package dto

import (
	"github.com/google/uuid"
	appdto "github.com/kipitix/growscada/internal/application/dto"
)

// TagResponse - HTTP DTO для представления тега в ответах API.
type TagResponse struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
	Version int       `json:"version"`
}

// GetTagsResponse - HTTP DTO для ответа со списком тегов.
type GetTagsResponse struct {
	Tags []TagResponse `json:"tags"`
}

// CreateTagRequest - HTTP DTO для запроса создания тега.
type CreateTagRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	Quality string `json:"quality"`
}

// CreateTagResponse - HTTP DTO для ответа на создание тега.
type CreateTagResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateTagRequest - HTTP DTO для запроса обновления значения тега.
type UpdateTagRequest struct {
	Value   string `json:"value"`
	Quality string `json:"quality"`
}

// UpdateTagResponse - HTTP DTO для ответа на обновление значения тега.
type UpdateTagResponse struct {
	Version int `json:"version"`
}

// DeleteTagResponse - HTTP DTO для ответа на удаление тега.
type DeleteTagResponse struct {
	Tag TagResponse `json:"tag"`
}

// NewTagResponse converts an app-level Tag DTO to a TagResponse HTTP DTO.
func NewTagResponse(t appdto.Tag) TagResponse {
	return TagResponse{
		ID:      t.ID,
		Name:    t.Name,
		Type:    t.Type,
		Value:   t.Value,
		Quality: t.Quality,
		Version: t.Version,
	}
}

// NewGetTagsResponse converts an app-level TagList DTO to a GetTagsResponse HTTP DTO.
func NewGetTagsResponse(list appdto.TagList) GetTagsResponse {
	tags := make([]TagResponse, len(list.Tags))
	for i, t := range list.Tags {
		tags[i] = NewTagResponse(t)
	}
	return GetTagsResponse{Tags: tags}
}
