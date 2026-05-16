package application

import (
	"context"
	"fmt"

	"github.com/kipitix/growscada/internal/application/appdto"
	"github.com/kipitix/growscada/internal/domain/event"
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/version"
)

// TagService is the service interface for working with tags.
type TagService interface {
	FindAllTags(context.Context) ([]appdto.Tag, error)
	FindTagByID(context.Context, id.ID[tag.Tag]) (appdto.Tag, error)
	CreateTag(context.Context, appdto.CreateTagInput) (appdto.Tag, error)
	DeleteTagByID(context.Context, id.ID[tag.Tag]) (appdto.Tag, error)
	SetTagValueByID(context.Context, appdto.UpdateTagInput) (appdto.Tag, error)
}

// tagServiceImpl is the tag service implementation.
type tagServiceImpl struct {
	tagRepository tag.TagRepository
	eventBus      event.EventBus
}

var _ TagService = (*tagServiceImpl)(nil)

// NewTagService creates and returns a new tag service instance.
// Returns a TagService interface implementation.
func NewTagService(aTagRepository tag.TagRepository, anEventBus event.EventBus) TagService {
	return &tagServiceImpl{
		tagRepository: aTagRepository,
		eventBus:      anEventBus,
	}
}

// FindAllTags returns a list of all tags
func (t tagServiceImpl) FindAllTags(ctx context.Context) ([]appdto.Tag, error) {
	tags, err := t.tagRepository.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find tags in repository: %w", err)
	}

	return appdto.NewTagList(tags), nil
}

// FindTagByID returns a tag by its identifier
func (t tagServiceImpl) FindTagByID(ctx context.Context, tagID id.ID[tag.Tag]) (appdto.Tag, error) {
	foundTag, err := t.tagRepository.FindByID(ctx, tagID)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("error on find tag by id in repository: %w", err)
	}

	return appdto.NewTag(foundTag), nil
}

// CreateTag creates a new tag and returns the created tag DTO
func (t tagServiceImpl) CreateTag(ctx context.Context, newTagData appdto.CreateTagInput) (appdto.Tag, error) {
	newTagID := t.tagRepository.NextID()

	newTagName, err := tag.NewTagName(newTagData.Name)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag because of name: %w", err)
	}

	newTagType, err := tag.NewTagType(newTagData.Type)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag because of type: %w", err)
	}

	newTagValue, err := newTagType.NewTagValue(newTagData.Value)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag because of value: %w", err)
	}

	newTagQuality, err := tag.NewTagQuality(newTagData.Quality)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag because of quality: %w", err)
	}

	newTag, err := tag.NewTag(newTagID, newTagName, newTagType, newTagValue, newTagQuality, version.Initial[tag.Tag]())
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag: %w", err)
	}

	newTag, err = t.tagRepository.Save(ctx, newTag)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot save tag: %w", err)
	}

	t.eventBus.Publish(event.NewTagCreatedEvent(newTagID))

	return appdto.NewTag(newTag), nil
}

// DeleteTagByID deletes a tag by its identifier and returns the deleted tag
func (t tagServiceImpl) DeleteTagByID(ctx context.Context, tagIDToDelete id.ID[tag.Tag]) (appdto.Tag, error) {
	deletedTag, err := t.tagRepository.DeleteByID(ctx, tagIDToDelete)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot delete tag: %w", err)
	}

	t.eventBus.Publish(event.NewTagDeletedEvent(tagIDToDelete))

	return appdto.NewTag(deletedTag), nil
}

// SetTagValueByID updates the value and quality of an existing tag and returns the updated tag
func (t tagServiceImpl) SetTagValueByID(ctx context.Context, request appdto.UpdateTagInput) (appdto.Tag, error) {
	tagIDToUpdate, err := id.NewID(id.IDWithUUID[tag.Tag](request.ID))
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot build tag id: %w", err)
	}

	foundTag, err := t.tagRepository.FindByID(ctx, tagIDToUpdate)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("error on find tag by id in repository: %w", err)
	}

	newQuality, err := tag.NewTagQuality(request.Quality)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot parse quality: %w", err)
	}

	if err = foundTag.SetValue(request.Value, newQuality); err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot set tag value: %w", err)
	}

	foundTag, err = t.tagRepository.Save(ctx, foundTag)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot save tag: %w", err)
	}

	t.eventBus.Publish(event.NewTagUpdatedEvent(tagIDToUpdate))

	return appdto.NewTag(foundTag), nil
}
