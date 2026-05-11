package restdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/application/appdto"
)

// TagResponse is the HTTP DTO for representing a tag in API responses.
type TagResponse struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
	Version int       `json:"version"`
}

// GetTagsResponse is the HTTP DTO for a list of tags response.
type GetTagsResponse struct {
	Tags []TagResponse `json:"tags"`
}

// CreateTagRequest is the HTTP DTO for a tag creation request.
type CreateTagRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	Quality string `json:"quality"`
}

// CreateTagResponse is the HTTP DTO for a tag creation response.
type CreateTagResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateTagRequest is the HTTP DTO for a tag value update request.
type UpdateTagRequest struct {
	Value   string `json:"value"`
	Quality string `json:"quality"`
}

// UpdateTagResponse is the HTTP DTO for a tag value update response.
type UpdateTagResponse struct {
	Version int `json:"version"`
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
func NewGetTagsResponse(list []appdto.Tag) GetTagsResponse {
	tags := make([]TagResponse, len(list))
	for i, t := range list {
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
