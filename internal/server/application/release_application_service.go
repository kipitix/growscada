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
	"github.com/kipitix/growscada/internal/server/domain/release"
)

// ReleaseService is the Engineering node's service of Project Releases (task
// 72_3; ADR 0021): a Commit is published as an immutable, numbered Release, the
// Releases are listed and read, a note is changed, a Release is downloaded as
// the ProjectFile it copied, and a Release is made the Project's running
// Deployment (task 72_4). It serves Projects alone: a Library's Releases are
// the LibraryService's (task 72_5).
type ReleaseService interface {
	// Create publishes Commit commitNumber of the live Project projectID as
	// the next Release, with the note and the Author. It needs no Draft
	// Version: a Commit is immutable. Returns an error matching
	// release.ErrCommitAlreadyReleased if the Commit is published already,
	// draft.ErrCommitNotFound if the Project has no such Commit, and
	// project.ErrProjectNotFound if the Project is not live.
	Create(ctx context.Context, projectID uuid.UUID, commitNumber int, note, author string) (appdto.Release, error)
	// CreateFromFile validates document as a project file (ADR 0018), publishes
	// it as a Release with origin file and deploys it (task 72_4): a file
	// deployment leaves a Release behind, so the history shows what runs. A
	// failure of the Deploy leaves the Release unpublished to the Runtime node
	// — it is immutable and stays for another attempt — and is returned with
	// it.
	CreateFromFile(ctx context.Context, projectID uuid.UUID, document []byte, author, title string) (appdto.Release, appdto.Deployment, error)
	// Deploy makes Release releaseNumber of the live Project its running
	// Deployment. Returns ErrNothingToDeploy if that Release runs already and
	// release.ErrReleaseNotFound for a number the Project does not have.
	Deploy(ctx context.Context, projectID uuid.UUID, releaseNumber int, author string) (appdto.Deployment, error)
	// List returns the Project's Releases, newest first, without their
	// content. It reads no Runtime node: a Release list is the Engineering
	// node's, and the running one is the Deployment's to name.
	List(ctx context.Context, projectID uuid.UUID) ([]appdto.Release, error)
	// Find returns one Release; a number the Project does not have is
	// release.ErrReleaseNotFound.
	Find(ctx context.Context, projectID uuid.UUID, number int) (appdto.Release, error)
	// UpdateNote changes the note of a Release. It is metadata: no Draft
	// Version is stated and the content never changes.
	UpdateNote(ctx context.Context, projectID uuid.UUID, number int, note string) (appdto.Release, error)
	// ProjectFile returns the ProjectFile a Release copied, as a download.
	ProjectFile(ctx context.Context, projectID uuid.UUID, number int) (ProjectFileDownload, error)
}

type releaseServiceImpl struct {
	content         ProjectContent
	releases        release.Repository
	runtime         RuntimeNode
	libraries       library.LibraryRepository
	libraryReleases library.LibraryReleaseRepository
}

var _ ReleaseService = (*releaseServiceImpl)(nil)

// NewReleaseService returns the service of the Releases of the Projects
// aContent serves, stored by someReleases, deployed through aRuntime and
// validating a project file against this server's Libraries and Releases.
func NewReleaseService(aContent ProjectContent, someReleases release.Repository, aRuntime RuntimeNode,
	someLibraries library.LibraryRepository, someLibraryReleases library.LibraryReleaseRepository) ReleaseService {
	return &releaseServiceImpl{content: aContent, releases: someReleases, runtime: aRuntime,
		libraries: someLibraries, libraryReleases: someLibraryReleases}
}

func (s releaseServiceImpl) Create(ctx context.Context, rawProjectID uuid.UUID, rawCommit int, rawNote, rawAuthor string) (appdto.Release, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Release{}, err
	}
	number, err := commitNumber(rawCommit)
	if err != nil {
		return appdto.Release{}, err
	}
	note, err := release.NewNote(rawNote)
	if err != nil {
		return appdto.Release{}, invalidInput(err)
	}
	author, err := draft.NewAuthor(rawAuthor)
	if err != nil {
		return appdto.Release{}, invalidInput(err)
	}
	scope := s.content.ScopeOf(projectID)
	commit, err := s.content.Drafts().FindCommit(ctx, scope, number)
	if err != nil {
		return appdto.Release{}, err
	}
	created, err := s.releases.Create(ctx, scope, commit, note, author)
	if err != nil {
		return appdto.Release{}, fmt.Errorf("cannot release commit %d of project %s: %w", number, projectID, err)
	}
	return appdto.NewRelease(created), nil
}

func (s releaseServiceImpl) CreateFromFile(ctx context.Context, rawProjectID uuid.UUID, document []byte, rawAuthor, rawTitle string) (appdto.Release, appdto.Deployment, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Release{}, appdto.Deployment{}, err
	}
	validator := projectFileValidator{libraries: s.libraries, releases: s.libraryReleases}
	validated, err := validator.validate(ctx, projectID, document, false)
	if err != nil {
		return appdto.Release{}, appdto.Deployment{}, err
	}
	author := rawAuthor
	if author == "" {
		// A file deployment has no account behind it: the Author tells where
		// the content came from, and it is the file.
		author = "file"
	}
	authorVO, err := draft.NewAuthor(author)
	if err != nil {
		return appdto.Release{}, appdto.Deployment{}, invalidInput(err)
	}
	title := rawTitle
	if title == "" {
		// The request carries no file name (the body is the document): a
		// plain title stands for it.
		title = "File"
	}
	titleVO, err := draft.NewCommitTitle(title)
	if err != nil {
		return appdto.Release{}, appdto.Deployment{}, invalidInput(err)
	}
	note, err := release.NewNote("")
	if err != nil {
		return appdto.Release{}, appdto.Deployment{}, invalidInput(err)
	}
	created, err := s.releases.CreateFromFile(ctx, s.content.ScopeOf(projectID), titleVO, note, authorVO,
		draft.NewProjectCommitContent(validated.file))
	if err != nil {
		return appdto.Release{}, appdto.Deployment{}, fmt.Errorf("cannot release the project file of %s: %w", projectID, err)
	}
	deployed, err := s.deploy(ctx, projectID, created.Number().Int(), validated.file)
	if err != nil {
		return appdto.NewRelease(created), appdto.Deployment{}, err
	}
	return appdto.NewRelease(created), deployed, nil
}

func (s releaseServiceImpl) Deploy(ctx context.Context, rawProjectID uuid.UUID, rawNumber int, _ string) (appdto.Deployment, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Deployment{}, err
	}
	number, err := releaseNumber(rawNumber)
	if err != nil {
		return appdto.Deployment{}, err
	}
	found, err := s.releases.Find(ctx, s.content.ScopeOf(projectID), number)
	if err != nil {
		return appdto.Deployment{}, err
	}
	file, ok := found.Content().ProjectFile()
	if !ok {
		return appdto.Deployment{}, fmt.Errorf("release %d of project %s holds no project file", number, projectID)
	}
	return s.deploy(ctx, projectID, number, file)
}

// deploy hands the document of Release number to the Runtime node: the one
// place both a Deploy of a Release and the Deploy of a file Release reach it.
func (s releaseServiceImpl) deploy(ctx context.Context, projectID id.ID[project.Project], number int,
	file projectv0.ProjectFile) (appdto.Deployment, error) {
	deployed, err := s.runtime.DeployRelease(ctx, projectID.UUID(), number, file)
	if err != nil {
		if errors.Is(err, ErrNothingToDeploy) {
			return appdto.Deployment{}, fmt.Errorf("%w: release %d of project %s runs already", ErrNothingToDeploy, number, projectID)
		}
		return appdto.Deployment{}, fmt.Errorf("cannot deploy release %d of project %s: %w", number, projectID, err)
	}
	return deployed, nil
}

func (s releaseServiceImpl) List(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.Release, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	found, err := s.releases.List(ctx, s.content.ScopeOf(projectID))
	if err != nil {
		return nil, err
	}
	return appdto.NewReleaseList(found), nil
}

func (s releaseServiceImpl) Find(ctx context.Context, rawProjectID uuid.UUID, rawNumber int) (appdto.Release, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Release{}, err
	}
	number, err := releaseNumber(rawNumber)
	if err != nil {
		return appdto.Release{}, err
	}
	found, err := s.releases.Find(ctx, s.content.ScopeOf(projectID), number)
	if err != nil {
		return appdto.Release{}, err
	}
	return appdto.NewRelease(found), nil
}

func (s releaseServiceImpl) UpdateNote(ctx context.Context, rawProjectID uuid.UUID, rawNumber int, rawNote string) (appdto.Release, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Release{}, err
	}
	number, err := releaseNumber(rawNumber)
	if err != nil {
		return appdto.Release{}, err
	}
	note, err := release.NewNote(rawNote)
	if err != nil {
		return appdto.Release{}, invalidInput(err)
	}
	updated, err := s.releases.UpdateNote(ctx, s.content.ScopeOf(projectID), number, note)
	if err != nil {
		return appdto.Release{}, err
	}
	return appdto.NewRelease(updated), nil
}

func (s releaseServiceImpl) ProjectFile(ctx context.Context, rawProjectID uuid.UUID, rawNumber int) (ProjectFileDownload, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	number, err := releaseNumber(rawNumber)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	found, err := s.releases.Find(ctx, s.content.ScopeOf(projectID), number)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	file, ok := found.Content().ProjectFile()
	if !ok {
		return ProjectFileDownload{}, fmt.Errorf("release %d of project %s holds no project file", number, projectID)
	}
	name, err := projectDownloadName(ctx, s.content, projectID)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	return ProjectFileDownload{File: file, ProjectName: name}, nil
}

// releaseNumber parses the number of a Release a client names; a number below
// 1 is the client's input, not a missing Release.
func releaseNumber(raw int) (int, error) {
	if raw < 1 {
		return 0, invalidInput(fmt.Errorf("release number must be positive, got %d", raw))
	}
	return raw, nil
}

// projectDownloadName returns the name of the live Project projectID, which
// names a download.
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
