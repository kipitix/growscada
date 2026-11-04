package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// DeployService is the Engineering node's Deploy (task 35, Q6, Q8, Q9): it
// assembles the ProjectFile of the Draft, compares it with the Revision the
// Project runs, and hands it to the Runtime node. It keeps no copy of what it
// sent: the Revision is the only truth (ADR 0014).
type DeployService interface {
	// Deploy deploys the Draft of the live Project projectID and returns the
	// Revision the Runtime node created.
	// Returns ErrNothingToDeploy if the Draft is the current Revision
	// unchanged, an error wrapping project.ErrProjectDiscarding while a
	// Discard of the Project runs, and an error wrapping
	// project.ErrProjectNotFound if the Project is not live.
	Deploy(ctx context.Context, projectID uuid.UUID) (appdto.Revision, error)
	// Preview reports what Deploy would do, without deploying: the current
	// Revision (absent until the first Deploy), whether the Draft is it
	// unchanged, and the runtime Widgets a Deploy would drop.
	Preview(ctx context.Context, projectID uuid.UUID) (appdto.DeployPreview, error)
}

// EditLockGate refuses a write of a Project's Draft whose EditLock the request
// does not hold (ADR 0015).
type EditLockGate interface {
	// CheckHeld returns editlock.ErrNoClient when the request names no client,
	// and editlock.ErrEditLockNotHeld when the lock is free or another
	// client's. The server's own writes pass.
	CheckHeld(ctx context.Context, projectID uuid.UUID) error
}

type deployServiceImpl struct {
	content ProjectContent
	builder ProjectFileBuilder
	runtime RuntimeNode
	locks   EditLockGate
}

var _ DeployService = (*deployServiceImpl)(nil)

// NewDeployService returns the DeployService over the content of the live
// Projects aContent, the builder aBuilder, the Runtime node aRuntime and the
// EditLocks of the Drafts aGate.
func NewDeployService(aContent ProjectContent, aBuilder ProjectFileBuilder, aRuntime RuntimeNode, aGate EditLockGate) DeployService {
	return &deployServiceImpl{content: aContent, builder: aBuilder, runtime: aRuntime, locks: aGate}
}

func (s deployServiceImpl) Deploy(ctx context.Context, projectID uuid.UUID) (appdto.Revision, error) {
	// The Draft is read-only while a Discard runs: half restored, it would go
	// into a Revision the policy of that Discard knows nothing of (Q10).
	if _, _, err := s.content.openNotDiscarding(ctx, projectID); err != nil {
		return appdto.Revision{}, err
	}
	// A Deploy is an act over the Draft as a whole: it needs the lock, checked
	// before the build, which spans several queries and has no single
	// transaction to check it in (ADR 0015).
	if err := s.locks.CheckHeld(ctx, projectID); err != nil {
		return appdto.Revision{}, err
	}
	file, err := s.builder.Build(ctx, projectID)
	if err != nil {
		return appdto.Revision{}, err
	}
	current, ok, err := s.runtime.FindCurrentRevision(ctx, projectID)
	if err != nil {
		return appdto.Revision{}, fmt.Errorf("cannot read the current revision of project %s: %w", projectID, err)
	}
	if ok {
		unchanged, err := sameProjectFile(file, current.Content)
		if err != nil {
			return appdto.Revision{}, err
		}
		if unchanged {
			return appdto.Revision{}, fmt.Errorf("%w: the draft of project %s is the revision it runs", ErrNothingToDeploy, projectID)
		}
	}

	revision, err := s.runtime.Deploy(ctx, projectID, file)
	if err != nil {
		return appdto.Revision{}, fmt.Errorf("cannot deploy project %s: %w", projectID, err)
	}
	return revision, nil
}

func (s deployServiceImpl) Preview(ctx context.Context, projectID uuid.UUID) (appdto.DeployPreview, error) {
	file, err := s.builder.Build(ctx, projectID)
	if err != nil {
		return appdto.DeployPreview{}, err
	}
	current, ok, err := s.runtime.FindCurrentRevision(ctx, projectID)
	if err != nil {
		return appdto.DeployPreview{}, fmt.Errorf("cannot read the current revision of project %s: %w", projectID, err)
	}
	preview := appdto.DeployPreview{DroppedWidgets: []appdto.DroppedWidget{}}
	if !ok {
		return preview, nil
	}
	preview.CurrentRevision = &current
	unchanged, err := sameProjectFile(file, current.Content)
	if err != nil {
		return appdto.DeployPreview{}, err
	}
	preview.Unchanged = unchanged
	return preview, nil
}
