package app_dto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/tag"
)

// Tag is the application-layer DTO for tag data. Fields have no JSON tags — serialization is handled at the interface layer.
type Tag struct {
	ID      uuid.UUID
	Name    string
	Type    string
	Value   string
	Quality string
	Version int
}

// CreateTagInput holds the input data for creating a tag.
type CreateTagInput struct {
	Name    string
	Type    string
	Value   string
	Quality string
}

// UpdateTagInput holds the input data for updating a tag's value and quality.
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
		Version: aTag.Version().Number(),
	}
}

// NewTagList creates a TagList DTO from a list of tag.Tag domain aggregates
func NewTagList(aTagList []tag.Tag) []Tag {
	tags := make([]Tag, len(aTagList))
	for i, t := range aTagList {
		tags[i] = NewTag(t)
	}
	return tags
}
