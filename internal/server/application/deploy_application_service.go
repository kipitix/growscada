package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
)

// ProjectFileCopies are the Engineering node's copies of the last ProjectFile
// it sent to the Runtime node (task 35, Q6): engineering.project_files, one per
// Project. A copy is secondary — the truth is the Revision — and it is what
// the Project mode shows and what tells the Draft that is deployed from one
// that is not.
type ProjectFileCopies interface {
	// Save replaces the copy of the Project's last ProjectFile. A write of an
	// older Revision than the stored one changes nothing.
	Save(ctx context.Context, projectID uuid.UUID, revision appdto.Revision) error
	// FindByProject returns the copy of the Project's last ProjectFile, and
	// whether the Project has one at all.
	FindByProject(ctx context.Context, projectID uuid.UUID) (appdto.Revision, bool, error)
}

// DeployService is the Engineering node's Deploy (task 35, Q6, Q8, Q9): it
// assembles the ProjectFile of the Draft, compares it with the Revision the
// Project runs, hands it to the Runtime node and keeps the copy of what it
// sent.
type DeployService interface {
	// Deploy deploys the Draft of the live Project projectID and returns the
	// Revision the Runtime node created.
	// Returns ErrNothingToDeploy if the Draft is the current Revision
	// unchanged, ErrRevisionCopyNotSaved if the Revision is created but its
	// copy is not saved, an error wrapping project.ErrProjectDiscarding while
	// a Discard of the Project runs, and an error wrapping
	// project.ErrProjectNotFound if the Project is not live.
	Deploy(ctx context.Context, projectID uuid.UUID) (appdto.Revision, error)
	// Preview reports what Deploy would do, without deploying: the current
	// Revision (absent until the first Deploy), whether the Draft is it
	// unchanged, and the runtime Widgets a Deploy would drop.
	Preview(ctx context.Context, projectID uuid.UUID) (appdto.DeployPreview, error)
	// State reports the Revision of the Engineering node's copy (absent until
	// a Deploy saved one) and whether the Draft differs from it.
	State(ctx context.Context, projectID uuid.UUID) (appdto.DeployState, error)
}

type deployServiceImpl struct {
	content ProjectContent
	builder ProjectFileBuilder
	files   ProjectFileCopies
	runtime RuntimeNode
}

var _ DeployService = (*deployServiceImpl)(nil)

// NewDeployService returns the DeployService over the content of the live
// Projects aContent, the builder aBuilder, the copies aFiles and the Runtime
// node aRuntime.
func NewDeployService(aContent ProjectContent, aBuilder ProjectFileBuilder, aFiles ProjectFileCopies, aRuntime RuntimeNode) DeployService {
	return &deployServiceImpl{content: aContent, builder: aBuilder, files: aFiles, runtime: aRuntime}
}

func (s deployServiceImpl) Deploy(ctx context.Context, projectID uuid.UUID) (appdto.Revision, error) {
	// The Draft is read-only while a Discard runs: half restored, it would go
	// into a Revision the policy of that Discard knows nothing of (Q10).
	if _, _, err := s.content.openNotDiscarding(ctx, projectID); err != nil {
		return appdto.Revision{}, err
	}
	file, err := s.builder.Build(ctx, projectID)
	if err != nil {
		return appdto.Revision{}, err
	}
	unchanged, err := s.unchanged(ctx, projectID, file)
	if err != nil {
		return appdto.Revision{}, err
	}
	if unchanged {
		return appdto.Revision{}, fmt.Errorf("%w: the draft of project %s is the revision it runs", ErrNothingToDeploy, projectID)
	}

	revision, err := s.runtime.Deploy(ctx, projectID, file)
	if err != nil {
		return appdto.Revision{}, fmt.Errorf("cannot deploy project %s: %w", projectID, err)
	}
	if err := s.files.Save(ctx, projectID, revision); err != nil {
		return appdto.Revision{}, fmt.Errorf("%w: revision %d of project %s is active: %w",
			ErrRevisionCopyNotSaved, revision.Number, projectID, err)
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

func (s deployServiceImpl) State(ctx context.Context, projectID uuid.UUID) (appdto.DeployState, error) {
	file, err := s.builder.Build(ctx, projectID)
	if err != nil {
		return appdto.DeployState{}, err
	}
	copied, ok, err := s.files.FindByProject(ctx, projectID)
	if err != nil {
		return appdto.DeployState{}, fmt.Errorf("cannot read the project file copy of %s: %w", projectID, err)
	}
	if !ok {
		return appdto.DeployState{Differs: true}, nil
	}
	unchanged, err := sameProjectFile(file, copied.Content)
	if err != nil {
		return appdto.DeployState{}, err
	}
	return appdto.DeployState{Deployed: &copied, Differs: !unchanged}, nil
}

// unchanged reports whether file is the Revision the Project runs. It compares
// with what the Runtime node holds, not with the Engineering node's copy
// (Q8): the copy may be missing after a Deploy whose write failed, and the
// Draft then must not pass for unchanged.
func (s deployServiceImpl) unchanged(ctx context.Context, projectID uuid.UUID, file projectv0.ProjectFile) (bool, error) {
	current, ok, err := s.runtime.FindCurrentRevision(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("cannot read the current revision of project %s: %w", projectID, err)
	}
	if !ok {
		return false, nil
	}
	return sameProjectFile(file, current.Content)
}
