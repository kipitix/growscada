package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// ProjectFileDownload is a ProjectFile as a download: the canonical document
// and the Project's name, which names the file (ADR 0018).
type ProjectFileDownload struct {
	File        projectv0.ProjectFile
	ProjectName string
}

// ProjectFileService is the Engineering node's service of project files
// (task 39, ADR 0018): exporting the Draft or a Deployment, opening a file as
// the Draft, and deploying a file as a Deployment without the Draft.
type ProjectFileService interface {
	// Draft returns the ProjectFile of the live Project's Draft, assembled by
	// the same builder a Deploy uses. A Draft the builder refuses is an error
	// matching ErrDraftNotDeployable, and an error wrapping
	// project.ErrProjectNotFound if the Project is not live.
	Draft(ctx context.Context, projectID uuid.UUID) (ProjectFileDownload, error)
	// Deployment returns the document the Runtime node stored for Deployment n of
	// the live Project. Returns runtimeapp.ErrDeploymentNotFound if there is
	// none and an error wrapping project.ErrProjectNotFound if the Project is
	// not live.
	Deployment(ctx context.Context, projectID uuid.UUID, n int) (ProjectFileDownload, error)
	// Open replaces the whole Draft with document, stating the Version the
	// client saw: one DraftChange, "Open a project
	// file" (ADR 0017). Returns an error matching ErrNothingToOpen if the
	// canonical document is the Draft already, and one matching
	// ErrInvalidProjectFile when the document does not validate.
	Open(ctx context.Context, projectID uuid.UUID, version int, document []byte) (appdto.DraftState, error)
	// Deploy makes document the new Deployment of the live Project without the
	// Draft (ADR 0018). Returns an error matching
	// ErrNothingToDeploy if the document is the running Deployment's.
	Deploy(ctx context.Context, projectID uuid.UUID, document []byte) (appdto.Deployment, error)
}

type projectFileServiceImpl struct {
	content   ProjectContent
	builder   ProjectFileBuilder
	runtime   RuntimeNode
	libraries library.LibraryRepository
	releases  library.LibraryReleaseRepository
}

var _ ProjectFileService = (*projectFileServiceImpl)(nil)

// NewProjectFileService returns the service of the project files of the live
// Projects aContent serves, reading the running Deployments through aRuntime and
// this server's Releases through the Libraries for opening a file.
func NewProjectFileService(aContent ProjectContent, aBuilder ProjectFileBuilder, aRuntime RuntimeNode,
	someLibraries library.LibraryRepository, someReleases library.LibraryReleaseRepository) ProjectFileService {
	return &projectFileServiceImpl{
		content: aContent, builder: aBuilder, runtime: aRuntime,
		libraries: someLibraries, releases: someReleases,
	}
}

func (s projectFileServiceImpl) Draft(ctx context.Context, rawProjectID uuid.UUID) (ProjectFileDownload, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	file, err := s.builder.Build(ctx, rawProjectID)
	if err != nil {
		if errors.Is(err, project.ErrProjectNotFound) {
			return ProjectFileDownload{}, err
		}
		return ProjectFileDownload{}, DraftNotDeployableError{Reason: err.Error()}
	}
	name, err := s.projectName(ctx, projectID)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	return ProjectFileDownload{File: file, ProjectName: name}, nil
}

func (s projectFileServiceImpl) Deployment(ctx context.Context, rawProjectID uuid.UUID, n int) (ProjectFileDownload, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	deployment, err := s.runtime.FindDeployment(ctx, projectID.UUID(), n)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	name, err := s.projectName(ctx, projectID)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	return ProjectFileDownload{File: deployment.Content, ProjectName: name}, nil
}

func (s projectFileServiceImpl) Open(ctx context.Context, rawProjectID uuid.UUID, rawVersion int, document []byte) (appdto.DraftState, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.DraftState{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.DraftState{}, err
	}
	validator := projectFileValidator{libraries: s.libraries, releases: s.releases}
	validated, err := validator.validate(ctx, projectID, document, true)
	if err != nil {
		return appdto.DraftState{}, err
	}

	// A file whose canonical form is the Draft already is nothing to open —
	// unless the Draft cannot be built at all: then a valid file repairs it
	// (ADR 0018).
	current, buildErr := s.builder.Build(ctx, rawProjectID)
	if buildErr == nil {
		same, err := sameProjectFile(current, validated.file)
		if err != nil {
			return appdto.DraftState{}, err
		}
		if same {
			return appdto.DraftState{}, fmt.Errorf("%w: the draft of project %s is the file already", ErrNothingToOpen, projectID)
		}
	}

	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Open a project file",
		func(session *draft.Session) error {
			if err := s.lockLocalPins(ctx, validated.file, session); err != nil {
				return err
			}
			return session.Open(ctx, validated.content)
		})
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot open the project file of %s: %w", projectID, err)
	}
	return appdto.DraftState{Version: result.Version().Number()}, nil
}

// lockLocalPins rechecks the local Releases inside the Draft's transaction:
// every pinned Library's row is locked FOR SHARE, as PutLibraryPin does, and
// its Release must still be there (ADR 0018). A Library deleted between the
// validation and the write is a file issue, not a pin to a deleted Library.
func (s projectFileServiceImpl) lockLocalPins(ctx context.Context, file projectv0.ProjectFile, session *draft.Session) error {
	var issues []FileIssue
	for i, pin := range file.LibraryPins {
		path := fmt.Sprintf("library_pins[%d]", i)
		libraryID := id.NewID(id.IDWithUUID[library.Library](pin.LibraryID))
		if err := session.LockLibrary(ctx, libraryID); err != nil {
			if errors.Is(err, library.ErrLibraryNotFound) {
				issues = append(issues, FileIssue{Path: path, Message: fmt.Sprintf("library %s is not live on this server", pin.LibraryID)})
				continue
			}
			return err
		}
		number, err := library.NewReleaseNumber(pin.Release)
		if err != nil {
			issues = append(issues, FileIssue{Path: path + ".release", Message: err.Error()})
			continue
		}
		if _, err := s.releases.FindByNumber(ctx, libraryID, number); err != nil {
			if errors.Is(err, library.ErrReleaseNotFound) {
				issues = append(issues, FileIssue{Path: path,
					Message: fmt.Sprintf("library %s has no release %d on this server", pin.LibraryID, pin.Release)})
				continue
			}
			return err
		}
	}
	if len(issues) > 0 {
		return ProjectFileError{Issues: issues}
	}
	return nil
}

func (s projectFileServiceImpl) Deploy(ctx context.Context, rawProjectID uuid.UUID, document []byte) (appdto.Deployment, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Deployment{}, err
	}
	// A deployed file is self-contained: the Runtime node runs the copies in
	// it and reads no Library, so the local ones are not checked (ADR 0018).
	validator := projectFileValidator{}
	validated, err := validator.validate(ctx, projectID, document, false)
	if err != nil {
		return appdto.Deployment{}, err
	}
	current, ok, err := s.runtime.FindCurrentDeployment(ctx, projectID.UUID())
	if err != nil {
		return appdto.Deployment{}, fmt.Errorf("cannot read the current deployment of project %s: %w", projectID, err)
	}
	if ok {
		same, err := sameProjectFile(validated.file, current.Content)
		if err != nil {
			return appdto.Deployment{}, err
		}
		if same {
			return appdto.Deployment{}, fmt.Errorf("%w: project %s runs the file already", ErrNothingToDeploy, projectID)
		}
	}
	deployment, err := s.runtime.DeployFile(ctx, projectID.UUID(), validated.file)
	if err != nil {
		return appdto.Deployment{}, fmt.Errorf("cannot deploy the project file to %s: %w", projectID, err)
	}
	return deployment, nil
}

// projectName returns the name of the live Project projectID, which names the
// downloaded file.
func (s projectFileServiceImpl) projectName(ctx context.Context, projectID id.ID[project.Project]) (string, error) {
	found, err := s.content.projects.FindByID(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("cannot read project %s: %w", projectID, err)
	}
	if found.OrganizationID() != s.content.organization {
		return "", id.NotFound(project.ErrProjectNotFound, projectID)
	}
	return found.Name().String(), nil
}

