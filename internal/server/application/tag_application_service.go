package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// TagService is the service interface for working with the tags of a
// project: their definitions in the Draft, each read with its state from the
// Runtime node (ADR 0011); a state is written there, not here. Every method
// takes the project first; a project that does not exist (or is deleted) is
// an error wrapping project.ErrProjectNotFound.
type TagService interface {
	FindAllTags(ctx context.Context, projectID uuid.UUID) ([]appdto.Tag, error)
	FindTagByID(ctx context.Context, projectID, tagID uuid.UUID) (appdto.Tag, error)
	FindTagByName(ctx context.Context, projectID uuid.UUID, name string) (appdto.Tag, error)
	FindTagsByNamePattern(ctx context.Context, projectID uuid.UUID, pattern string) ([]appdto.Tag, error)
	FindTagsByNameRegex(ctx context.Context, projectID uuid.UUID, expr string) ([]appdto.Tag, error)
	CreateTag(ctx context.Context, projectID uuid.UUID, input appdto.CreateTagInput) (appdto.Tag, error)
	DeleteTagByID(ctx context.Context, projectID, tagID uuid.UUID) (appdto.Tag, error)
}

// tagServiceImpl is the tag service implementation.
type tagServiceImpl struct {
	content ProjectContent
	runtime RuntimeNode
}

var _ TagService = (*tagServiceImpl)(nil)

// NewTagService creates and returns a new tag service instance. The events
// of its changes are recorded by the Tag and saved with it (ADR 0008); the
// states of the tags are read from aRuntime.
// Returns a TagService interface implementation.
func NewTagService(aContent ProjectContent, aRuntime RuntimeNode) TagService {
	return &tagServiceImpl{content: aContent, runtime: aRuntime}
}

// withStates returns the DTOs of the tags of the project, each with its
// state from the Runtime node, read in one call; a tag the Runtime node has
// no state of yet (its definition not upserted yet) shows its initial one,
// and so does every tag when the Runtime node fails to answer: the Draft is
// read without it.
func (t tagServiceImpl) withStates(ctx context.Context, projectID uuid.UUID, tags []tag.Tag) []appdto.Tag {
	if len(tags) == 0 {
		return []appdto.Tag{}
	}
	ids := make([]uuid.UUID, len(tags))
	for i, found := range tags {
		ids[i] = found.ID().UUID()
	}
	states, err := t.runtime.FindTagStates(ctx, projectID, ids)
	if err != nil {
		slog.Warn("Tag states not read from the runtime node; the tags show their initial ones",
			"project", projectID, "error", err)
	}
	byID := make(map[uuid.UUID]appdto.TagState, len(states))
	for _, st := range states {
		byID[st.ID] = st
	}
	result := make([]appdto.Tag, len(tags))
	for i, found := range tags {
		st, ok := byID[found.ID().UUID()]
		if !ok {
			st = appdto.InitialTagState(found)
		}
		result[i] = appdto.NewTag(found, st)
	}
	return result
}

// withState returns the DTO of one tag with its state.
func (t tagServiceImpl) withState(ctx context.Context, projectID uuid.UUID, found tag.Tag) appdto.Tag {
	return t.withStates(ctx, projectID, []tag.Tag{found})[0]
}

// tags returns the tag repository of the live project rawProjectID.
func (t tagServiceImpl) tags(ctx context.Context, rawProjectID uuid.UUID) (tag.TagRepository, error) {
	_, scope, err := t.content.open(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	return scope.Tags(), nil
}

// FindAllTags returns a list of all tags of the project. The UI reloads it on
// every event: the tags are read with the check of their project in one
// query, and the project is read apart only when there are none, to tell a
// missing project from an empty one.
func (t tagServiceImpl) FindAllTags(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.Tag, error) {
	_, scope := t.content.openUnchecked(rawProjectID)
	tags, err := scope.Tags().FindAllInLiveProject(ctx, t.content.organization)
	if err != nil {
		return nil, fmt.Errorf("error on find tags in repository: %w", err)
	}
	if len(tags) == 0 {
		if _, _, err := t.content.open(ctx, rawProjectID); err != nil {
			return nil, err
		}
	}

	return t.withStates(ctx, rawProjectID, tags), nil
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

	return t.withState(ctx, rawProjectID, foundTag), nil
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

	return t.withState(ctx, rawProjectID, foundTag), nil
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
	return t.withStates(ctx, rawProjectID, matched), nil
}

// CreateTag creates a new tag in the project and returns the created tag DTO,
// with its initial state. A project deleted just after it was read here is
// not found by the save. Right after commit it gives the Runtime node the
// tag's definition (giveRuntimeDefinition), so the tag accepts values once
// this returns; if that fails, CreateTag returns the error although the tag
// is created, and NewRuntimeTagDefinitionsPolicy gives the definition on
// tag_created.
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

	newTag, err := tag.CreateTag(newTagID, projectID, newTagName, newTagType)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag: %w", err)
	}

	newTag, err = repo.Save(ctx, newTag)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot save tag: %w", err)
	}

	if err := t.giveRuntimeDefinition(ctx, rawProjectID, repo, newTag); err != nil {
		return appdto.Tag{}, fmt.Errorf("tag %s created, but not given to the runtime node yet: %w", newTag.ID(), err)
	}

	return appdto.NewTag(newTag, appdto.InitialTagState(newTag)), nil
}

// runtimeCallTimeout bounds a call to the Runtime node made after commit.
const runtimeCallTimeout = 10 * time.Second

// giveRuntimeDefinition upserts the definition of a tag just created into
// the Runtime node, then reads the tag from the Draft again: if it has been
// deleted meanwhile, its state is removed. The deletion's own removal and the
// job of NewRuntimeTagDefinitionsPolicy may both have run before the upsert,
// and nothing would remove the state left by it. A deletion committed after
// the read removes the state itself, after the upsert. The calls run apart
// from the caller's cancellation: the tag is committed already.
func (t tagServiceImpl) giveRuntimeDefinition(ctx context.Context, rawProjectID uuid.UUID, repo tag.TagRepository, created tag.Tag) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runtimeCallTimeout)
	defer cancel()

	if err := t.runtime.UpsertTagDefinitions(ctx, rawProjectID, []appdto.TagDefinition{appdto.NewTagDefinition(created)}); err != nil {
		return err
	}
	_, err := repo.FindByID(ctx, created.ID())
	if errors.Is(err, tag.ErrTagNotFound) {
		if err := t.runtime.RemoveTagStates(ctx, rawProjectID, []uuid.UUID{created.ID().UUID()}); err != nil {
			return fmt.Errorf("cannot remove the state of tag deleted meanwhile: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read tag back: %w", err)
	}
	return nil
}

// DeleteTagByID deletes a tag by its identifier and returns the deleted tag,
// as it was read, with its last state. It expects no Version, and the
// repository deletes the tag whatever its stored Version. Right after commit
// it removes the tag's state from the Runtime node; if that fails,
// NewRuntimeTagDefinitionsPolicy removes it on tag_deleted.
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
	last := t.withState(ctx, rawProjectID, found)
	if err := repo.Delete(ctx, found.Delete()); err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot delete tag: %w", err)
	}

	if err := t.runtime.RemoveTagStates(ctx, rawProjectID, []uuid.UUID{rawID}); err != nil {
		slog.Warn("Tag state not removed from the runtime node; the policy on tag_deleted removes it",
			"project", rawProjectID, "tag", rawID, "error", err)
	}

	return last, nil
}
