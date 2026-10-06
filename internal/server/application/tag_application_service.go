package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// TagService is the service interface for working with tags.
type TagService interface {
	FindAllTags(context.Context) ([]appdto.Tag, error)
	FindTagByID(context.Context, uuid.UUID) (appdto.Tag, error)
	FindTagByName(context.Context, string) (appdto.Tag, error)
	FindTagsByNamePattern(context.Context, string) ([]appdto.Tag, error)
	FindTagsByNameRegex(context.Context, string) ([]appdto.Tag, error)
	CreateTag(context.Context, appdto.CreateTagInput) (appdto.Tag, error)
	DeleteTagByID(context.Context, uuid.UUID) (appdto.Tag, error)
	SetTagValueByID(context.Context, appdto.UpdateTagInput) (appdto.Tag, error)
}

// tagServiceImpl is the tag service implementation.
type tagServiceImpl struct {
	tagRepository tag.TagRepository
}

var _ TagService = (*tagServiceImpl)(nil)

// NewTagService creates and returns a new tag service instance. The events
// of its changes are recorded by the Tag and saved with it (ADR 0008).
// Returns a TagService interface implementation.
func NewTagService(aTagRepository tag.TagRepository) TagService {
	return &tagServiceImpl{tagRepository: aTagRepository}
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
func (t tagServiceImpl) FindTagByID(ctx context.Context, rawID uuid.UUID) (appdto.Tag, error) {
	tagID := id.NewID(id.IDWithUUID[tag.Tag](rawID))
	foundTag, err := t.tagRepository.FindByID(ctx, tagID)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("error on find tag by id in repository: %w", err)
	}

	return appdto.NewTag(foundTag), nil
}

// FindTagByName returns a tag by its unique name
func (t tagServiceImpl) FindTagByName(ctx context.Context, rawName string) (appdto.Tag, error) {
	tagName, err := tag.NewTagName(rawName)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot find tag because of name: %w", invalidInput(err))
	}

	foundTag, err := t.tagRepository.FindByName(ctx, tagName)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("error on find tag by name in repository: %w", err)
	}

	return appdto.NewTag(foundTag), nil
}

// FindTagsByNamePattern returns the tags whose whole name matches a wildcard
// pattern ("*" any sequence, "?" one character). An invalid pattern returns
// an error wrapping tag.ErrInvalidTagNameMatcher and ErrInvalidInput.
func (t tagServiceImpl) FindTagsByNamePattern(ctx context.Context, pattern string) ([]appdto.Tag, error) {
	matcher, err := tag.NewTagNamePattern(pattern)
	if err != nil {
		return nil, fmt.Errorf("cannot find tags because of name pattern: %w", invalidInput(err))
	}
	return t.findTagsMatching(ctx, matcher)
}

// FindTagsByNameRegex returns the tags whose name matches a Go (RE2) regular
// expression anywhere. An invalid expression returns an error wrapping
// tag.ErrInvalidTagNameMatcher and ErrInvalidInput.
func (t tagServiceImpl) FindTagsByNameRegex(ctx context.Context, expr string) ([]appdto.Tag, error) {
	matcher, err := tag.NewTagNameRegex(expr)
	if err != nil {
		return nil, fmt.Errorf("cannot find tags because of name regex: %w", invalidInput(err))
	}
	return t.findTagsMatching(ctx, matcher)
}

func (t tagServiceImpl) findTagsMatching(ctx context.Context, matcher tag.TagNameMatcher) ([]appdto.Tag, error) {
	tags, err := t.tagRepository.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find tags in repository: %w", err)
	}
	var matched []tag.Tag
	for _, found := range tags {
		if matcher.Matches(found.Name()) {
			matched = append(matched, found)
		}
	}
	return appdto.NewTagList(matched), nil
}

// CreateTag creates a new tag and returns the created tag DTO
func (t tagServiceImpl) CreateTag(ctx context.Context, newTagData appdto.CreateTagInput) (appdto.Tag, error) {
	newTagID := t.tagRepository.NextID()

	newTagName, err := tag.NewTagName(newTagData.Name)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag because of name: %w", invalidInput(err))
	}

	newTagType, err := tag.NewTagType(newTagData.Type)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag because of type: %w", invalidInput(err))
	}

	newTagValue, err := newTagType.NewTagValue(newTagData.Value)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag because of value: %w", invalidInput(err))
	}

	newTagQuality, err := tag.NewTagQuality(newTagData.Quality)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag because of quality: %w", invalidInput(err))
	}

	newTag, err := tag.CreateTag(newTagID, newTagName, newTagType, newTagValue, newTagQuality)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag: %w", err)
	}

	newTag, err = t.tagRepository.Save(ctx, newTag)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot save tag: %w", err)
	}

	return appdto.NewTag(newTag), nil
}

// DeleteTagByID deletes a tag by its identifier and returns the deleted tag.
// It expects no Version: a tag changed meanwhile is reread and deleted as it
// is now (retryOnRace).
func (t tagServiceImpl) DeleteTagByID(ctx context.Context, rawID uuid.UUID) (appdto.Tag, error) {
	tagID := id.NewID(id.IDWithUUID[tag.Tag](rawID))

	var deleted tag.Tag
	err := retryOnRace(tag.ErrTagConflict, func() error {
		found, err := t.tagRepository.FindByID(ctx, tagID)
		if err != nil {
			return fmt.Errorf("error on find tag by id in repository: %w", err)
		}
		found.Delete()
		if err := t.tagRepository.Delete(ctx, found); err != nil {
			return fmt.Errorf("cannot delete tag: %w", err)
		}
		deleted = found
		return nil
	})
	if err != nil {
		return appdto.Tag{}, err
	}

	return appdto.NewTag(deleted), nil
}

// SetTagValueByID updates the value and quality of an existing tag and returns the updated tag
func (t tagServiceImpl) SetTagValueByID(ctx context.Context, request appdto.UpdateTagInput) (appdto.Tag, error) {
	tagID := id.NewID(id.IDWithUUID[tag.Tag](request.ID))

	foundTag, err := t.tagRepository.FindByID(ctx, tagID)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("error on find tag by id in repository: %w", err)
	}

	// The Version check stays here, not in Tag.SetValue: a process value has
	// a single writer, and its Version goes with task 32.
	if foundTag.Version().Number() != request.Version {
		return appdto.Tag{}, tag.ErrTagConflict
	}

	newQuality, err := tag.NewTagQuality(request.Quality)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot parse quality: %w", invalidInput(err))
	}

	// The stored tag's type is valid (FindByID rejects any other), so SetValue
	// fails only when the caller's value does not fit that type.
	if err = foundTag.SetValue(request.Value, newQuality); err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot set tag value: %w", invalidInput(err))
	}

	foundTag, err = t.tagRepository.Save(ctx, foundTag)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot save tag: %w", err)
	}

	return appdto.NewTag(foundTag), nil
}
