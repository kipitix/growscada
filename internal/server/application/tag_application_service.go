package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// TagService is the service interface for working with the tags of a
// project: their definitions in the Draft, each read with its state from the
// Runtime node (ADR 0011); a state is written there, not here. Every write of
// the Draft's content states its Version (ADR 0016). Every method takes the
// project first; a project that does not exist (or is deleted) is an error
// wrapping project.ErrProjectNotFound.
type TagService interface {
	FindAllTags(ctx context.Context, projectID uuid.UUID) ([]appdto.Tag, error)
	FindTagByID(ctx context.Context, projectID, tagID uuid.UUID) (appdto.Tag, error)
	FindTagByName(ctx context.Context, projectID uuid.UUID, name string) (appdto.Tag, error)
	FindTagsByNamePattern(ctx context.Context, projectID uuid.UUID, pattern string) ([]appdto.Tag, error)
	FindTagsByNameRegex(ctx context.Context, projectID uuid.UUID, expr string) ([]appdto.Tag, error)
	CreateTag(ctx context.Context, projectID uuid.UUID, input appdto.CreateTagInput) (appdto.Tag, error)
	DeleteTagByID(ctx context.Context, projectID, tagID uuid.UUID, version int) (appdto.Tag, error)
}

// tagServiceImpl is the tag service implementation.
type tagServiceImpl struct {
	content ProjectContent
	runtime RuntimeNode
}

var _ TagService = (*tagServiceImpl)(nil)

// NewTagService creates and returns a new tag service instance. The events of
// its changes are recorded by the Tag and saved with the Draft (ADR 0008,
// ADR 0016); the states of the tags are read from aRuntime.
func NewTagService(aContent ProjectContent, aRuntime RuntimeNode) TagService {
	return &tagServiceImpl{content: aContent, runtime: aRuntime}
}

// withStates returns the DTOs of the tags of the project, each with its
// state from the Runtime node, read in one call; a tag the Runtime node has
// no state of yet (its definition not upserted yet) shows its initial one,
// and so does every tag when the Runtime node fails to answer: the Draft is
// read without it. version is the Version of the Project's Draft.
func (t tagServiceImpl) withStates(ctx context.Context, projectID uuid.UUID, tags []tag.Tag, version int) []appdto.Tag {
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
		result[i] = appdto.NewTag(found, st, version)
	}
	return result
}

// withState returns the DTO of one tag with its state.
func (t tagServiceImpl) withState(ctx context.Context, projectID uuid.UUID, found tag.Tag, version int) appdto.Tag {
	return t.withStates(ctx, projectID, []tag.Tag{found}, version)[0]
}

// tags returns the tag read repository of the live project rawProjectID.
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
	version, err := t.content.Version(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	return t.withStates(ctx, rawProjectID, tags, version), nil
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
	version, err := t.content.Version(ctx, rawProjectID)
	if err != nil {
		return appdto.Tag{}, err
	}
	return t.withState(ctx, rawProjectID, foundTag, version), nil
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
	version, err := t.content.Version(ctx, rawProjectID)
	if err != nil {
		return appdto.Tag{}, err
	}
	return t.withState(ctx, rawProjectID, foundTag, version), nil
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
	version, err := t.content.Version(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	return t.withStates(ctx, rawProjectID, matched, version), nil
}

// CreateTag creates a new tag in the project and returns the created tag DTO,
// with its initial state. The write states the Version of the Draft the
// client saw: a stale one is draft.ErrDraftConflict. The definition reaches
// the Runtime node with the next Deploy: until then the Tag runs no state, and
// a write of its value is not found (ADR 0011, task 35).
func (t tagServiceImpl) CreateTag(ctx context.Context, rawProjectID uuid.UUID, newTagData appdto.CreateTagInput) (appdto.Tag, error) {
	projectID, _, err := t.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Tag{}, err
	}
	newTagName, err := tag.NewTagName(newTagData.Name)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag because of name: %w", invalidInput(err))
	}
	newTagType, err := tag.NewTagType(newTagData.Type)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag because of type: %w", invalidInput(err))
	}
	expected, err := draftVersion(newTagData.Version)
	if err != nil {
		return appdto.Tag{}, err
	}
	newTagID := id.NewID[tag.Tag]()
	newTag, err := tag.CreateTag(newTagID, projectID, newTagName, newTagType, tag.NoSupplier())
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot create tag: %w", err)
	}
	result, err := t.content.Drafts().Update(ctx, t.content.ScopeOf(projectID), expected, "Create tag "+newTagName.String(),
		func(session *draft.Session) error {
			session.CreateTag(ctx, newTag)
			return nil
		})
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot save tag: %w", err)
	}
	return appdto.NewTag(newTag, appdto.InitialTagState(newTag), result.Version().Number()), nil
}

// DeleteTagByID deletes a tag by its identifier and returns the deleted tag,
// as it was read, with its last state. The write states the Version of the
// Draft the client saw and, in the same change, removes the Tag's PortBindings
// from every Scene of the Project (ADR 0010, ADR 0017). The Runtime node's copy
// of the definition stays until the next Deploy removes it with the Deployment
// it came from (ADR 0011, task 35): nothing of the Draft writes there.
func (t tagServiceImpl) DeleteTagByID(ctx context.Context, rawProjectID, rawID uuid.UUID, rawVersion int) (appdto.Tag, error) {
	projectID, scope, err := t.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Tag{}, err
	}
	tagID := id.NewID(id.IDWithUUID[tag.Tag](rawID))
	found, err := scope.Tags().FindByID(ctx, tagID)
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("error on find tag by id in repository: %w", err)
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Tag{}, err
	}
	var deleted tag.Tag
	result, err := t.content.Drafts().Update(ctx, t.content.ScopeOf(projectID), expected, "Delete tag "+found.Name().String(),
		func(session *draft.Session) error {
			var err error
			deleted, err = session.DeleteTag(ctx, tagID)
			return err
		})
	if err != nil {
		return appdto.Tag{}, fmt.Errorf("cannot delete tag: %w", err)
	}
	return t.withState(ctx, rawProjectID, deleted, result.Version().Number()), nil
}

// draftVersion parses the Version of the Draft a client states; a negative
// one is no version at all: the client's input, not a conflict.
func draftVersion(raw int) (version.Version[draft.Draft], error) {
	expected, err := version.New(version.WithNumber[draft.Draft](raw))
	if err != nil {
		return expected, invalidInput(fmt.Errorf("version: %w", err))
	}
	return expected, nil
}
