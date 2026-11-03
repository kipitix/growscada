package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// DiscardService is the Engineering node's Discard (task 35, Q10): it marks the
// Project as being reset to the Revision it runs and answers at once; the
// policy of NewDiscardProjectPolicy brings the Draft back.
type DiscardService interface {
	// Discard marks the live Project projectID as being reset to the Revision
	// it runs and returns it marked.
	// Returns ErrNoRevisionToDiscard if the Project has no Revision, and an
	// error wrapping project.ErrProjectNotFound if it is not live.
	Discard(ctx context.Context, projectID uuid.UUID) (appdto.Project, error)
}

type discardServiceImpl struct {
	content ProjectContent
	runtime RuntimeNode
}

var _ DiscardService = (*discardServiceImpl)(nil)

// NewDiscardService returns the DiscardService of the live Projects the
// ProjectContent serves, reading the current Revision through the Runtime node
// aRuntime.
func NewDiscardService(aContent ProjectContent, aRuntime RuntimeNode) DiscardService {
	return &discardServiceImpl{content: aContent, runtime: aRuntime}
}

func (s discardServiceImpl) Discard(ctx context.Context, rawProjectID uuid.UUID) (appdto.Project, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Project{}, err
	}
	if _, ok, err := s.runtime.FindCurrentRevision(ctx, projectID.UUID()); err != nil {
		return appdto.Project{}, fmt.Errorf("cannot read the current revision of project %s: %w", projectID, err)
	} else if !ok {
		return appdto.Project{}, fmt.Errorf("%w: %s", ErrNoRevisionToDiscard, projectID)
	}

	var discarded project.Project
	err = retryOnRace(project.ErrProjectConflict, func() error {
		found, err := findOwnProject(ctx, s.content.projects, s.content.organization, projectID)
		if err != nil {
			return err
		}
		marked, err := found.StartDiscard()
		if err != nil {
			return fmt.Errorf("cannot discard project %s: %w", projectID, err)
		}
		saved, err := s.content.projects.Save(ctx, marked)
		if err != nil {
			return fmt.Errorf("cannot save project %s: %w", projectID, err)
		}
		discarded = saved
		return nil
	})
	if err != nil {
		return appdto.Project{}, err
	}
	return appdto.NewProject(discarded), nil
}
