package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// TagService is the service interface for working with the tags of a
// project. Every method takes the project first; a project that does not
// exist (or is deleted) is an error wrapping project.ErrProjectNotFound.
type TagService interface {
	FindAllTags(ctx context.Context, projectID uuid.UUID) ([]appdto.Tag, error)
	FindTagByID(ctx context.Context, projectID, tagID uuid.UUID) (appdto.Tag, error)
	FindTagByName(ctx context.Context, projectID uuid.UUID, name string) (appdto.Tag, error)
	FindTagsByNamePattern(ctx context.Context, projectID uuid.UUID, pattern string) ([]appdto.Tag, error)
	FindTagsByNameRegex(ctx context.Context, projectID uuid.UUID, expr string) ([]appdto.Tag, error)
	CreateTag(ctx context.Context, projectID uuid.UUID, input appdto.CreateTagInput) (appdto.Tag, error)
	DeleteTagByID(ctx context.Context, projectID, tagID uuid.UUID) (appdto.Tag, error)
	SetTagValueByID(ctx context.Context, projectID uuid.UUID, input appdto.UpdateTagInput) (appdto.Tag, error)
}

// tagServiceImpl is the tag service implementation.
type tagServiceImpl struct {
	content ProjectContent
}

var _ TagService = (*tagServiceImpl)(nil)

// NewTagService creates and returns a new tag service instance. The events
// of its changes are recorded by the Tag and saved with it (ADR 0008).
// Returns a TagService interface implementation.
func NewTagService(aContent ProjectContent) TagService {
	return &tagServiceImpl{content: aContent}
}

// tags returns the tag repository of the live project rawProjectID.
func (t tagServiceImpl) tags(ctx context.Context, rawProjectID uuid.UUID) (tag.TagRepository, error) {
	_, scope, err := t.content.open(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	return scope.Tags(), nil
}

// FindAllTags returns a list of all tags of the project
func (t tagServiceImpl) FindAllTags(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.Tag, error) {
	repo, err := t.tags(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	tags, err := repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find tags in repository: %w", err)
	}

	return appdto.NewTagList(tags), nil
}

// FindTagByID returns a tag by its identifier
func (t tagServiceImpl) FindTagByID(ctx context.Context, rawProjectID, rawID uuid.UUID) (appdto.Tag, error) {
	repo, err := t.tags(ctx, rawProjectID)
	if err != nil {
		return appdto.Tag{}, err
	}
	tagID := id.NewID(id.IDWithUUID[tag.Tag](rawID))
	foundTag, err := repo.FindByID(ctx, tagID)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("error on find tag by id in repository: %w", err)
	}

	return appdto.NewTag(foundTag), nil
}

// FindTagByName returns a tag by its name, unique within the project
func (t tagServiceImpl) FindTagByName(ctx context.Context, rawProjectID uuid.UUID, rawName string) (appdto.Tag, error) {
	repo, err := t.tags(ctx, rawProjectID)
	if err != nil {
		return appdto.Tag{}, err
	}
	tagName, err := tag.NewTagName(rawName)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot find tag because of name: %w", invalidInput(err))
	}

	foundTag, err := repo.FindByName(ctx, tagName)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("error on find tag by name in repository: %w", err)
	}

	return appdto.NewTag(foundTag), nil
}

// FindTagsByNamePattern returns the tags whose whole name matches a wildcard
// pattern ("*" any sequence, "?" one character). An invalid pattern returns
// an error wrapping tag.ErrInvalidTagNameMatcher and ErrInvalidInput.
func (t tagServiceImpl) FindTagsByNamePattern(ctx context.Context, rawProjectID uuid.UUID, pattern string) ([]appdto.Tag, error) {
	matcher, err := tag.NewTagNamePattern(pattern)
	if err != nil {
		return nil, fmt.Errorf("cannot find tags because of name pattern: %w", invalidInput(err))
	}
	return t.findTagsMatching(ctx, rawProjectID, matcher)
}

// FindTagsByNameRegex returns the tags whose name matches a Go (RE2) regular
// expression anywhere. An invalid expression returns an error wrapping
// tag.ErrInvalidTagNameMatcher and ErrInvalidInput.
func (t tagServiceImpl) FindTagsByNameRegex(ctx context.Context, rawProjectID uuid.UUID, expr string) ([]appdto.Tag, error) {
	matcher, err := tag.NewTagNameRegex(expr)
	if err != nil {
		return nil, fmt.Errorf("cannot find tags because of name regex: %w", invalidInput(err))
	}
	return t.findTagsMatching(ctx, rawProjectID, matcher)
}

func (t tagServiceImpl) findTagsMatching(ctx context.Context, rawProjectID uuid.UUID, matcher tag.TagNameMatcher) ([]appdto.Tag, error) {
	repo, err := t.tags(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	tags, err := repo.FindAll(ctx)
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

// CreateTag creates a new tag in the project and returns the created tag DTO.
// A project deleted just after it was read here is not found by the save.
func (t tagServiceImpl) CreateTag(ctx context.Context, rawProjectID uuid.UUID, newTagData appdto.CreateTagInput) (appdto.Tag, error) {
	projectID, scope, err := t.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Tag{}, err
	}
	repo := scope.Tags()
	newTagID := repo.NextID()

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

	newTag, err := tag.CreateTag(newTagID, projectID, newTagName, newTagType, newTagValue, newTagQuality)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag: %w", err)
	}

	newTag, err = repo.Save(ctx, newTag)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot save tag: %w", err)
	}

	return appdto.NewTag(newTag), nil
}

// DeleteTagByID deletes a tag by its identifier and returns the deleted tag,
// as it was read. It expects no Version, and the repository deletes the tag
// whatever its stored Version: a value written meanwhile does not stop it.
func (t tagServiceImpl) DeleteTagByID(ctx context.Context, rawProjectID, rawID uuid.UUID) (appdto.Tag, error) {
	repo, err := t.tags(ctx, rawProjectID)
	if err != nil {
		return appdto.Tag{}, err
	}
	tagID := id.NewID(id.IDWithUUID[tag.Tag](rawID))

	found, err := repo.FindByID(ctx, tagID)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("error on find tag by id in repository: %w", err)
	}
	found.Delete()
	if err := repo.Delete(ctx, found); err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot delete tag: %w", err)
	}

	return appdto.NewTag(found), nil
}

// SetTagValueByID updates the value and quality of an existing tag and returns the updated tag.
// It is the hot path of Devices: the tag is read with the check of its
// project in one query, and the project is read apart only when the tag is
// not found, to tell a missing project from a missing tag.
func (t tagServiceImpl) SetTagValueByID(ctx context.Context, rawProjectID uuid.UUID, request appdto.UpdateTagInput) (appdto.Tag, error) {
	_, scope := t.content.openUnchecked(rawProjectID)
	repo := scope.Tags()
	tagID := id.NewID(id.IDWithUUID[tag.Tag](request.ID))

	foundTag, err := repo.FindByIDInLiveProject(ctx, tagID, t.content.organization)
	if errors.Is(err, tag.ErrTagNotFound) {
		if _, _, projectErr := t.content.open(ctx, rawProjectID); projectErr != nil {
			return appdto.Tag{}, projectErr
		}
	}
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

	foundTag, err = repo.Save(ctx, foundTag)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot save tag: %w", err)
	}

	return appdto.NewTag(foundTag), nil
}
