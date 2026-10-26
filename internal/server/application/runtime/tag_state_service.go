// Package runtime holds the use-case services of the Runtime node
// (ADR 0011): the Tag states. It never imports the Engineering node's
// packages, nor do they import it: the Engineering node reaches it through its
// RuntimeNode port, which the server wires to TagStateService in-process.
package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/tagstate"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// ErrInvalidInput matches (errors.Is) an error caused by the caller's input
// to the Runtime node, as application.ErrInvalidInput does for the
// Engineering node's.
var ErrInvalidInput = errors.New("invalid input")

type invalidInputError struct {
	err error
}

func (e invalidInputError) Error() string        { return e.err.Error() }
func (e invalidInputError) Unwrap() error        { return e.err }
func (e invalidInputError) Is(target error) bool { return target == ErrInvalidInput }

func invalidInput(err error) error {
	return invalidInputError{err: err}
}

// TagDefinition is the copy of a Tag definition the Runtime node is given.
type TagDefinition struct {
	ID      uuid.UUID
	Name    string
	Type    string
	Version int
}

// TagState is a Tag's state with its definition's copy. Seq 0: written by no
// supplier yet.
type TagState struct {
	ID      uuid.UUID
	Name    string
	Type    string
	Version int
	Value   string
	Quality string
	Seq     int64
}

// SetTagValueInput is a write of a Tag's state; it states no expected
// Version: the latest write wins.
type SetTagValueInput struct {
	TagID   uuid.UUID
	Value   string
	Quality string
}

// TagStateService is the Runtime node's service of Tag states. Every method
// takes the Project first.
type TagStateService interface {
	// SetTagValue writes a Tag's state and, once it is stored, publishes
	// tag_updated with it (at-most-once). A Project that is not live, or of
	// another Organization, is an error wrapping project.ErrProjectNotFound;
	// a Tag without a state one wrapping tag.ErrTagNotFound.
	SetTagValue(ctx context.Context, projectID uuid.UUID, input SetTagValueInput) (TagState, error)
	// FindTagStates returns the states of the Tags with the given IDs; a Tag
	// without a state is left out.
	FindTagStates(ctx context.Context, projectID uuid.UUID, tagIDs []uuid.UUID) ([]TagState, error)
	// UpsertTagDefinitions gives the Runtime node the definitions of Tags: a
	// Tag without a state gets its initial one (its TagType's zero value,
	// Quality bad), a Tag with one keeps it. Idempotent.
	UpsertTagDefinitions(ctx context.Context, projectID uuid.UUID, defs []TagDefinition) error
	// RemoveTagStates removes the states of the Tags; one without a state is
	// skipped.
	RemoveTagStates(ctx context.Context, projectID uuid.UUID, tagIDs []uuid.UUID) error
	// RemoveProjectTagStates removes the states of every Tag of the Project.
	RemoveProjectTagStates(ctx context.Context, projectID uuid.UUID) error
}

type tagStateServiceImpl struct {
	states       tagstate.Repository
	organization id.ID[organization.Organization]
	publish      func(event.Event)
}

var _ TagStateService = (*tagStateServiceImpl)(nil)

// NewTagStateService returns the service of the Tag states of the Projects
// of the Organization anOrganizationID; publish delivers tag_updated (the
// EventBus of the SSE streams).
func NewTagStateService(someStates tagstate.Repository, anOrganizationID id.ID[organization.Organization], publish func(event.Event)) TagStateService {
	return &tagStateServiceImpl{states: someStates, organization: anOrganizationID, publish: publish}
}

// SetTagValue is the hot path of Devices: the state is read with the check
// of its Project in one query, and the Project is read apart only when the
// Tag is not found, to tell a missing Project from a missing Tag.
func (s tagStateServiceImpl) SetTagValue(ctx context.Context, rawProjectID uuid.UUID, input SetTagValueInput) (TagState, error) {
	projectID := projectIDOf(rawProjectID)
	tagID := tagIDOf(input.TagID)

	current, err := s.states.FindInLiveProject(ctx, projectID, tagID, s.organization)
	if errors.Is(err, tag.ErrTagNotFound) {
		if projectErr := s.checkProject(ctx, projectID); projectErr != nil {
			return TagState{}, projectErr
		}
	}
	if err != nil {
		return TagState{}, fmt.Errorf("error on find tag state in repository: %w", err)
	}

	quality, err := tag.NewTagQuality(input.Quality)
	if err != nil {
		return TagState{}, fmt.Errorf("cannot parse quality: %w", invalidInput(err))
	}
	// The stored TagType is valid (the read rejects any other), so Write
	// fails only when the caller's value does not fit it.
	written, err := current.Write(input.Value, quality)
	if err != nil {
		return TagState{}, fmt.Errorf("cannot set tag value: %w", invalidInput(err))
	}
	stored, err := s.states.Write(ctx, written)
	if err != nil {
		return TagState{}, fmt.Errorf("cannot write tag state: %w", err)
	}
	s.publish(tagstate.NewTagUpdatedEvent(stored))
	return NewTagState(stored), nil
}

// checkProject returns an error wrapping project.ErrProjectNotFound unless
// the Project is live and of the server's Organization.
func (s tagStateServiceImpl) checkProject(ctx context.Context, projectID id.ID[project.Project]) error {
	live, err := s.states.IsProjectLive(ctx, projectID, s.organization)
	if err != nil {
		return err
	}
	if !live {
		return id.NotFound(project.ErrProjectNotFound, projectID)
	}
	return nil
}

func (s tagStateServiceImpl) FindTagStates(ctx context.Context, rawProjectID uuid.UUID, rawTagIDs []uuid.UUID) ([]TagState, error) {
	tagIDs := make([]id.ID[tag.Tag], len(rawTagIDs))
	for i, raw := range rawTagIDs {
		tagIDs[i] = tagIDOf(raw)
	}
	found, err := s.states.FindByIDs(ctx, projectIDOf(rawProjectID), tagIDs)
	if err != nil {
		return nil, fmt.Errorf("error on find tag states in repository: %w", err)
	}
	states := make([]TagState, 0, len(found))
	for _, tagID := range tagIDs {
		if st, ok := found[tagID]; ok {
			states = append(states, NewTagState(st))
		}
	}
	return states, nil
}

func (s tagStateServiceImpl) UpsertTagDefinitions(ctx context.Context, rawProjectID uuid.UUID, rawDefs []TagDefinition) error {
	projectID := projectIDOf(rawProjectID)
	defs := make([]tagstate.Definition, len(rawDefs))
	for i, raw := range rawDefs {
		def, err := newDefinition(projectID, raw)
		if err != nil {
			return fmt.Errorf("cannot upsert tag definition %s: %w", raw.ID, invalidInput(err))
		}
		defs[i] = def
	}
	if err := s.states.UpsertDefinitions(ctx, projectID, defs); err != nil {
		return fmt.Errorf("cannot upsert tag definitions: %w", err)
	}
	return nil
}

func (s tagStateServiceImpl) RemoveTagStates(ctx context.Context, rawProjectID uuid.UUID, rawTagIDs []uuid.UUID) error {
	tagIDs := make([]id.ID[tag.Tag], len(rawTagIDs))
	for i, raw := range rawTagIDs {
		tagIDs[i] = tagIDOf(raw)
	}
	if err := s.states.Delete(ctx, projectIDOf(rawProjectID), tagIDs); err != nil {
		return fmt.Errorf("cannot remove tag states: %w", err)
	}
	return nil
}

func (s tagStateServiceImpl) RemoveProjectTagStates(ctx context.Context, rawProjectID uuid.UUID) error {
	if err := s.states.DeleteAllOfProject(ctx, projectIDOf(rawProjectID)); err != nil {
		return fmt.Errorf("cannot remove tag states: %w", err)
	}
	return nil
}

func newDefinition(projectID id.ID[project.Project], raw TagDefinition) (tagstate.Definition, error) {
	name, err := tag.NewTagName(raw.Name)
	if err != nil {
		return tagstate.Definition{}, err
	}
	tagType, err := tag.NewTagType(raw.Type)
	if err != nil {
		return tagstate.Definition{}, err
	}
	ver, err := version.New(version.WithNumber[tag.Tag](raw.Version))
	if err != nil {
		return tagstate.Definition{}, err
	}
	return tagstate.Definition{TagID: tagIDOf(raw.ID), ProjectID: projectID, Name: name, Type: tagType, Version: ver}, nil
}

// NewTagState returns the DTO of a state; the interface layer translates
// tag_updated with it.
func NewTagState(s tagstate.TagState) TagState {
	def := s.Definition()
	return TagState{
		ID:      def.TagID.UUID(),
		Name:    def.Name.String(),
		Type:    def.Type.String(),
		Version: def.Version.Number(),
		Value:   s.Value().String(),
		Quality: s.Quality().String(),
		Seq:     int64(s.Seq()),
	}
}

func projectIDOf(raw uuid.UUID) id.ID[project.Project] {
	return id.NewID(id.IDWithUUID[project.Project](raw))
}

func tagIDOf(raw uuid.UUID) id.ID[tag.Tag] {
	return id.NewID(id.IDWithUUID[tag.Tag](raw))
}
