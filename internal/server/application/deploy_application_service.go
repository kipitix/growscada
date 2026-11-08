package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// DeployService is the Engineering node's Deploy (task 35, Q6, Q8, Q9): it
// assembles the ProjectFile of the Draft, compares it with the Deployment the
// Project runs, and hands it to the Runtime node. It keeps no copy of what it
// sent: the Deployment is the only truth (ADR 0014).
type DeployService interface {
	// Deploy deploys the Draft of the live Project projectID and returns the
	// Deployment the Runtime node created. After a successful Deploy its
	// DraftChange stack is cleared (ADR 0017).
	// Returns ErrNothingToDeploy if the Draft is the current Deployment
	// unchanged, and an error wrapping project.ErrProjectNotFound if the
	// Project is not live.
	Deploy(ctx context.Context, projectID uuid.UUID) (appdto.Deployment, error)
	// Preview reports what Deploy would do, without deploying: the current
	// Deployment (absent until the first Deploy), whether the Draft is it
	// unchanged, and the runtime Widgets a Deploy would drop.
	Preview(ctx context.Context, projectID uuid.UUID) (appdto.DeployPreview, error)
}

type deployServiceImpl struct {
	content    ProjectContent
	builder    ProjectFileBuilder
	runtime    RuntimeNode
	libraries  library.LibraryRepository
	releases   library.LibraryReleaseRepository
}

var _ DeployService = (*deployServiceImpl)(nil)

// NewDeployService returns the DeployService over the content of the live
// Projects aContent, the builder aBuilder and the Runtime node aRuntime. The
// Libraries and Releases are read for the preview: it names the pins of the
// running Deployment this server lacks (task 71).
func NewDeployService(aContent ProjectContent, aBuilder ProjectFileBuilder, aRuntime RuntimeNode,
	someLibraries library.LibraryRepository, someReleases library.LibraryReleaseRepository) DeployService {
	return &deployServiceImpl{content: aContent, builder: aBuilder, runtime: aRuntime,
		libraries: someLibraries, releases: someReleases}
}

func (s deployServiceImpl) Deploy(ctx context.Context, projectID uuid.UUID) (appdto.Deployment, error) {
	checked, _, err := s.content.open(ctx, projectID)
	if err != nil {
		return appdto.Deployment{}, err
	}
	// The Draft is read and built over several queries with no transaction
	// around them. The Version read before the build is checked again in a
	// short transaction after it, so the document sent is the Draft at that
	// Version and a write that landed meanwhile is a conflict, not a mixed
	// ProjectFile (ADR 0022).
	version, err := s.content.DraftVersion(ctx, checked)
	if err != nil {
		return appdto.Deployment{}, err
	}
	file, err := s.builder.Build(ctx, projectID)
	if err != nil {
		return appdto.Deployment{}, err
	}
	current, ok, err := s.runtime.FindCurrentDeployment(ctx, projectID)
	if err != nil {
		return appdto.Deployment{}, fmt.Errorf("cannot read the current deployment of project %s: %w", projectID, err)
	}
	if ok {
		unchanged, err := sameProjectFile(file, current.Content)
		if err != nil {
			return appdto.Deployment{}, err
		}
		if unchanged {
			return appdto.Deployment{}, fmt.Errorf("%w: the draft of project %s is the deployment it runs", ErrNothingToDeploy, projectID)
		}
	}
	if err := s.content.Drafts().CheckVersion(ctx, draft.ProjectScope(checked), version); err != nil {
		return appdto.Deployment{}, err
	}

	deployment, err := s.runtime.Deploy(ctx, projectID, file)
	if err != nil {
		return appdto.Deployment{}, fmt.Errorf("cannot deploy project %s: %w", projectID, err)
	}
	// The Deployment stands: the DraftChange stack that made it is cleared in a
	// transaction of its own, because Deploy crosses to the Runtime node. A
	// failure to clear is reported while the Deployment stands (ADR 0017).
	if err := s.content.Drafts().ClearStack(ctx, draft.ProjectScope(checked)); err != nil {
		return appdto.Deployment{}, fmt.Errorf("cannot clear the draft stack of deployed project %s: %w", projectID, err)
	}
	return deployment, nil
}

func (s deployServiceImpl) Preview(ctx context.Context, projectID uuid.UUID) (appdto.DeployPreview, error) {
	file, err := s.builder.Build(ctx, projectID)
	if err != nil {
		return appdto.DeployPreview{}, err
	}
	current, ok, err := s.runtime.FindCurrentDeployment(ctx, projectID)
	if err != nil {
		return appdto.DeployPreview{}, fmt.Errorf("cannot read the current deployment of project %s: %w", projectID, err)
	}
	preview := appdto.DeployPreview{
		DroppedWidgets:  []appdto.DroppedWidget{},
		MissingReleases: []appdto.MissingRelease{},
	}
	if !ok {
		return preview, nil
	}
	preview.CurrentDeployment = &current
	unchanged, err := sameProjectFile(file, current.Content)
	if err != nil {
		return appdto.DeployPreview{}, err
	}
	preview.Unchanged = unchanged
	// The pins of the running Deployment this server cannot write: a Discard
	// drops their Widgets instead of restoring them (task 71).
	missing, err := missingLibraryReleases(ctx, s.libraries, s.releases, current.Content)
	if err != nil {
		return appdto.DeployPreview{}, fmt.Errorf("cannot check the library pins of project %s: %w", projectID, err)
	}
	for libraryID, release := range missing {
		preview.MissingReleases = append(preview.MissingReleases, appdto.MissingRelease{LibraryID: libraryID, Release: release})
	}
	sort.Slice(preview.MissingReleases, func(i, j int) bool {
		return preview.MissingReleases[i].LibraryID.String() < preview.MissingReleases[j].LibraryID.String()
	})
	return preview, nil
}
