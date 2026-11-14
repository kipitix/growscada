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
	// Deploy validates document as a project file (ADR 0018) and makes it the
	// Project's new Deployment without touching the Draft, stating no saved
	// state: a file deployment leaves no saved state behind (task 85_4). Returns
	// an error matching ErrInvalidProjectFile when the document does not
	// validate, and project.ErrProjectNotFound if the Project is not live.
	Deploy(ctx context.Context, projectID uuid.UUID, document []byte) (appdto.Deployment, error)
}

type projectFileServiceImpl struct {
	content   ProjectContent
	builder   ProjectFileBuilder
	runtime   RuntimeNode
	libraries library.LibraryRepository
}

var _ ProjectFileService = (*projectFileServiceImpl)(nil)

// NewProjectFileService returns the service of the project files of the live
// Projects aContent serves, reading the running Deployments through aRuntime and
// this server's Libraries and their saved states — through aContent's Drafts —
// for opening a file.
func NewProjectFileService(aContent ProjectContent, aBuilder ProjectFileBuilder, aRuntime RuntimeNode,
	someLibraries library.LibraryRepository) ProjectFileService {
	return &projectFileServiceImpl{
		content: aContent, builder: aBuilder, runtime: aRuntime,
		libraries: someLibraries,
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
	name, err := projectDownloadName(ctx, s.content, projectID)
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
	name, err := projectDownloadName(ctx, s.content, projectID)
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
	validator := projectFileValidator{libraries: s.libraries, drafts: s.content.Drafts()}
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
			if err := s.lockLocalImports(ctx, validated.file, session); err != nil {
				return err
			}
			return session.Open(ctx, validated.content)
		})
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot open the project file of %s: %w", projectID, err)
	}
	return appdto.DraftState{Version: result.Version().Number(), HasUnsavedChanges: result.HasUnsavedChanges()}, nil
}

func (s projectFileServiceImpl) Deploy(ctx context.Context, rawProjectID uuid.UUID, document []byte) (appdto.Deployment, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Deployment{}, err
	}
	validator := projectFileValidator{libraries: s.libraries, drafts: s.content.Drafts()}
	validated, err := validator.validate(ctx, projectID, document, false)
	if err != nil {
		return appdto.Deployment{}, err
	}
	deployed, err := s.runtime.DeployFile(ctx, projectID.UUID(), validated.file)
	if err != nil {
		return appdto.Deployment{}, fmt.Errorf("cannot deploy the project file of %s: %w", projectID, err)
	}
	return deployed, nil
}

// lockLocalImports rechecks the local saved states inside the Draft's
// transaction: every imported Library's row is locked FOR SHARE, as
// PutLibraryImport does, and its Commit must still be there (ADR 0018,
// task 85_6). A Library deleted between the validation and the write is a file
// issue, not an import of a deleted Library.
func (s projectFileServiceImpl) lockLocalImports(ctx context.Context, file projectv0.ProjectFile, session *draft.Session) error {
	var issues []FileIssue
	for i, imported := range file.LibraryImports {
		path := fmt.Sprintf("library_imports[%d]", i)
		libraryID := id.NewID(id.IDWithUUID[library.Library](imported.LibraryID))
		if err := session.LockLibrary(ctx, libraryID); err != nil {
			if errors.Is(err, library.ErrLibraryNotFound) {
				issues = append(issues, FileIssue{Path: path, Message: fmt.Sprintf("library %s is not live on this server", imported.LibraryID)})
				continue
			}
			return err
		}
		if _, err := s.content.Drafts().FindCommit(ctx, draft.LibraryScope(libraryID), imported.Commit); err != nil {
			if errors.Is(err, draft.ErrCommitNotFound) {
				issues = append(issues, FileIssue{Path: path,
					Message: fmt.Sprintf("library %s has no saved state %d on this server", imported.LibraryID, imported.Commit)})
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

// projectDownloadName returns the name of the live Project projectID, which
// names a downloaded file of it: a Draft, a Deployment or a Commit (task 39,
// ADR 0018).
func projectDownloadName(ctx context.Context, content ProjectContent, projectID id.ID[project.Project]) (string, error) {
	found, err := content.projects.FindByID(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("cannot read project %s: %w", projectID, err)
	}
	if found.OrganizationID() != content.organization {
		return "", id.NotFound(project.ErrProjectNotFound, projectID)
	}
	return found.Name().String(), nil
}
