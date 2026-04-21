package application

import (
	"context"
	"fmt"

	"github.com/kipitix/growscada/internal/application/dto"
	"github.com/kipitix/growscada/internal/domain/tag"
)

// TagService - service interface for working with tags
type TagService interface {
	FindAllTags(context.Context) (dto.FindAllTagsResponse, error)
	FindTagByID(context.Context, int) (dto.Tag, error)
	CreateTag(context.Context, dto.CreateTagRequest) (dto.CreateTagResponse, error)
}

// tagServiceImpl - tag service implementation
type tagServiceImpl struct {
	tagRepository tag.TagRepository
}

var _ TagService = (*tagServiceImpl)(nil)

// NewTagService creates and returns a new tag service instance.
// Returns a TagService interface implementation.
func NewTagService(aTagRepository tag.TagRepository) TagService {
	return &tagServiceImpl{
		tagRepository: aTagRepository,
	}
}

// FindAllTags returns a list of all tags
func (t tagServiceImpl) FindAllTags(ctx context.Context) (dto.FindAllTagsResponse, error) {
	tags, err := t.tagRepository.FindAll(ctx)
	if err != nil {
		return dto.FindAllTagsResponse{}, fmt.Errorf("error on find tags in repository: %w", err)
	}

	return dto.NewFindAllTagsResponse(tags), nil
}

// FindTagByID returns a tag by its identifier
// TODO: implement
func (t tagServiceImpl) FindTagByID(ctx context.Context, id int) (dto.Tag, error) {
	return dto.Tag{}, nil
}

// CreateTag creates a new tag
func (t tagServiceImpl) CreateTag(ctx context.Context, newTagData dto.CreateTagRequest) (dto.CreateTagResponse, error) {
	newTagID := t.tagRepository.NextID()

	newTagName, err := tag.NewTagName(newTagData.Name)
	if err != nil {
		return dto.CreateTagResponse{}, fmt.Errorf("cannot create tag because of name: %w", err)
	}

	newTagKind, err := tag.NewTagKind(newTagData.Kind)
	if err != nil {
		return dto.CreateTagResponse{}, fmt.Errorf("cannot create tag because of kind: %w", err)
	}

	newTagValue, err := tag.NewTagValue(newTagData.Value, newTagKind)
	if err != nil {
		return dto.CreateTagResponse{}, fmt.Errorf("cannot create tag because of value: %w", err)
	}

	newTagQuality, err := tag.NewTagQuality(newTagData.Quality)
	if err != nil {
		return dto.CreateTagResponse{}, fmt.Errorf("cannot create tag because of quality: %w", err)
	}

	newTag, err := tag.NewTag(newTagID, newTagName, newTagKind, newTagValue, newTagQuality, tag.TagVersionInitial)
	if err != nil {
		return dto.CreateTagResponse{}, fmt.Errorf("cannot create tag: %w", err)
	}

	err = t.tagRepository.Save(ctx, newTag)
	if err != nil {
		return dto.CreateTagResponse{}, fmt.Errorf("cannot save tag: %w", err)
	}

	return dto.CreateTagResponse{ID: newTagID.UUID()}, nil
}
