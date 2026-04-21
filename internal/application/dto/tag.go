package dto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/tag"
)

// Tag - data structure for transferring tag information through the API.
// Contains the main tag attributes in a JSON-friendly format.
type Tag struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Kind    string    `json:"kind"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
	Version int       `json:"version"`
}

// FindTagByIDResponse - response structure for a single tag
type FindTagByIDResponse struct {
	Tag Tag `json:"tag"`
}

// FindAllTagsResponse - structure for transferring a list of tags.
// Used for returning a collection of tags in API responses.
type FindAllTagsResponse struct {
	Tags []Tag `json:"tags"`
}

// CreateTagRequest - request structure for creating a tag
type CreateTagRequest struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Value   string `json:"value"`
	Quality string `json:"quality"`
}

// CreateTagResponse - response structure for creating a tag
type CreateTagResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateTagRequest - request structure for updating a tag
type UpdateTagRequest struct {
	ID      uuid.UUID `json:"id"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
}

// UpdateTagResponse - response structure for updating a tag
type UpdateTagResponse struct {
	Version int `json:"version"`
}

// DeleteTagRequest - request structure for deleting a tag
type DeleteTagRequest struct {
	ID uuid.UUID `json:"id"`
}

// DeleteTagResponse - response structure for deleting a tag
type DeleteTagResponse struct {
	Tag Tag `json:"tag"`
}

// NewTag creates a Tag DTO from the tag.Tag domain aggregate
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

// NewFindAllTagsResponse creates a FindAllTagsResponse DTO from a list of tag.Tag domain aggregates
func NewFindAllTagsResponse(aTagList []tag.Tag) FindAllTagsResponse {
	tags := make([]Tag, len(aTagList))
	for i, t := range aTagList {
		tags[i] = NewTag(t)
	}
	return FindAllTagsResponse{
		Tags: tags,
	}
}
