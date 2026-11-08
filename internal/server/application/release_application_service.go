package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/release"
)

// ReleaseService is the Engineering node's service of Project Releases (task
// 72_3; ADR 0021): a Commit is published as an immutable, numbered Release, the
// Releases are listed and read, a note is changed, and a Release is downloaded
// as the ProjectFile it copied. It serves Projects alone; task 72_5 extends it
// to Libraries.
type ReleaseService interface {
	// Create publishes Commit commitNumber of the live Project projectID as
	// the next Release, with the note and the Author. It needs no Draft
	// Version: a Commit is immutable. Returns an error matching
	// release.ErrCommitAlreadyReleased if the Commit is published already,
	// draft.ErrCommitNotFound if the Project has no such Commit, and
	// project.ErrProjectNotFound if the Project is not live.
	Create(ctx context.Context, projectID uuid.UUID, commitNumber int, note, author string) (appdto.Release, error)
	// List returns the Project's Releases, newest first, without their
	// content.
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
	content  ProjectContent
	releases release.Repository
}

var _ ReleaseService = (*releaseServiceImpl)(nil)

// NewReleaseService returns the service of the Releases of the Projects
// aContent serves, stored by someReleases.
func NewReleaseService(aContent ProjectContent, someReleases release.Repository) ReleaseService {
	return &releaseServiceImpl{content: aContent, releases: someReleases}
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
