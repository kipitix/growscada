package restdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
)

// GetTagResponse - HTTP DTO для представления тега в ответах API.
type GetTagResponse struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
	Version int       `json:"version"`
}

// GetTagsResponse - HTTP DTO для ответа со списком тегов.
type GetTagsResponse struct {
	Tags []GetTagResponse `json:"tags"`
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
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
	Version int       `json:"version"`
}

// NewTagResponse converts an app-level Tag DTO to a TagResponse HTTP DTO.
func NewTagResponse(t appdto.Tag) GetTagResponse {
	return GetTagResponse{
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
	tags := make([]GetTagResponse, len(list.Tags))
	for i, t := range list.Tags {
		tags[i] = NewTagResponse(t)
	}
	return GetTagsResponse{Tags: tags}
}

// NewCreateTagResponse converts an app-level Tag DTO to a CreateTagResponse HTTP DTO.
func NewCreateTagResponse(t appdto.Tag) CreateTagResponse {
	return CreateTagResponse{ID: t.ID}
}

// NewUpdateTagResponse converts an app-level Tag DTO to an UpdateTagResponse HTTP DTO.
func NewUpdateTagResponse(t appdto.Tag) UpdateTagResponse {
	return UpdateTagResponse{Version: t.Version}
}

// NewDeleteTagResponse converts an app-level Tag DTO to a DeleteTagResponse HTTP DTO.
func NewDeleteTagResponse(t appdto.Tag) DeleteTagResponse {
	return DeleteTagResponse{
		ID:      t.ID,
		Name:    t.Name,
		Type:    t.Type,
		Value:   t.Value,
		Quality: t.Quality,
		Version: t.Version,
	}
}

// NewCreateTagInput converts a CreateTagRequest HTTP DTO to an app-level CreateTagInput DTO.
func NewCreateTagInput(r CreateTagRequest) appdto.CreateTagInput {
	return appdto.CreateTagInput{
		Name:    r.Name,
		Type:    r.Type,
		Value:   r.Value,
		Quality: r.Quality,
	}
}

// NewUpdateTagInput converts an UpdateTagRequest HTTP DTO to an app-level UpdateTagInput DTO.
func NewUpdateTagInput(r UpdateTagRequest, tagID uuid.UUID) appdto.UpdateTagInput {
	return appdto.UpdateTagInput{
		ID:      tagID,
		Value:   r.Value,
		Quality: r.Quality,
	}
}
