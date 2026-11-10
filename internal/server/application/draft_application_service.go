package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
)

// DraftService reads the state of a Project's Draft (ADR 0016): the Version
// every content write states and whether the Draft holds changes its last
// Commit does not. It is what a client that has read no content yet — a Project
// with no Scenes and no Tags — reads the Version from.
type DraftService interface {
	// State returns the Version of the live Project's Draft and its
	// unsaved-changes flag.
	State(ctx context.Context, projectID uuid.UUID) (appdto.DraftState, error)
}

type draftServiceImpl struct {
	content ProjectContent
}

var _ DraftService = (*draftServiceImpl)(nil)

// NewDraftService returns the service reading the Drafts' state.
func NewDraftService(aContent ProjectContent) DraftService {
	return &draftServiceImpl{content: aContent}
}

func (s draftServiceImpl) State(ctx context.Context, rawProjectID uuid.UUID) (appdto.DraftState, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.DraftState{}, err
	}
	found, ok, err := s.content.Drafts().Read(ctx, draft.ProjectScope(projectID))
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot read the draft of project %s: %w", projectID, err)
	}
	if !ok {
		// A Draft whose row does not exist yet is a fresh one at version 1: the
		// Version its first write states. It has no content to differ from a
		// Commit, so it is clean.
		return appdto.DraftState{Version: 1}, nil
	}
	return appdto.DraftState{Version: found.Version().Number(), HasUnsavedChanges: found.HasUnsavedChanges()}, nil
}
