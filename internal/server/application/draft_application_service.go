package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// DraftService reads the state of a Project's Draft (ADR 0016): the Version
// every content write states. It is what a client that has read no content
// yet — a Project with no Scenes and no Tags — reads the Version from.
type DraftService interface {
	// State returns the Version of the live Project's Draft.
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
	if _, _, err := s.content.open(ctx, rawProjectID); err != nil {
		return appdto.DraftState{}, err
	}
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot read the draft of project %s: %w", rawProjectID, err)
	}
	return appdto.DraftState{Version: version}, nil
}
