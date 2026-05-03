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
	FindTagByID(context.Context, tag.TagID) (dto.Tag, error)
	CreateTag(context.Context, dto.CreateTagRequest) (dto.CreateTagResponse, error)
	DeleteTagByID(context.Context, tag.TagID) (dto.DeleteTagResponse, error)
	SetTagValueByID(context.Context, dto.UpdateTagRequest) (dto.UpdateTagResponse, error)
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
func (t tagServiceImpl) FindTagByID(ctx context.Context, tagID tag.TagID) (dto.Tag, error) {
	foundTag, err := t.tagRepository.FindByID(ctx, tagID)
	if err != nil {
		return dto.Tag{}, fmt.Errorf("error on find tag by id in repository: %w", err)
	}

	return dto.NewTag(foundTag), nil
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

	newTagValue, err := newTagKind.NewTagValue(newTagData.Value)
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

// DeleteTagByID deletes a tag by its identifier and returns the deleted tag
func (t tagServiceImpl) DeleteTagByID(ctx context.Context, tagID tag.TagID) (dto.DeleteTagResponse, error) {
	deletedTag, err := t.tagRepository.DeleteByID(ctx, tagID)
	if err != nil {
		return dto.DeleteTagResponse{}, fmt.Errorf("cannot delete tag: %w", err)
	}

	return dto.DeleteTagResponse{Tag: dto.NewTag(deletedTag)}, nil
}

// SetTagValue updates the value and quality of an existing tag
func (t tagServiceImpl) SetTagValueByID(ctx context.Context, request dto.UpdateTagRequest) (dto.UpdateTagResponse, error) {
	tagID := tag.NewTagID(tag.TagIDWithUUID(request.ID))

	foundTag, err := t.tagRepository.FindByID(ctx, tagID)
	if err != nil {
		return dto.UpdateTagResponse{}, fmt.Errorf("error on find tag by id in repository: %w", err)
	}

	newQuality, err := tag.NewTagQuality(request.Quality)
	if err != nil {
		return dto.UpdateTagResponse{}, fmt.Errorf("cannot parse quality: %w", err)
	}

	if err = foundTag.SetValue(request.Value, newQuality); err != nil {
		return dto.UpdateTagResponse{}, fmt.Errorf("cannot set tag value: %w", err)
	}

	if err = t.tagRepository.Save(ctx, foundTag); err != nil {
		return dto.UpdateTagResponse{}, fmt.Errorf("cannot save tag: %w", err)
	}

	return dto.UpdateTagResponse{Version: foundTag.Version()}, nil
}
