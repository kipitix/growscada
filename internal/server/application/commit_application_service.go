package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/release"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// CommitService is the Engineering node's service of Commits (task 72_2;
// ADR 0021): a Save fixes the whole Draft as an immutable, numbered snapshot,
// the history is listed and read, and a title is renamed without touching the
// snapshot. It serves both a Project's Draft and a Library's, named by a
// draft.Scope.
type CommitService interface {
	// Save fixes the Draft of scope at version as a Commit with the title and
	// author, and returns it. Returns ErrNothingToSave if the Draft is at the
	// Version of its last Commit, a DraftNotSaveableError if the Draft cannot
	// be assembled, and draft.ConflictError if version is stale. An empty
	// title or author is invalid input.
	Save(ctx context.Context, scope draft.Scope, version int, title, author string) (appdto.Commit, error)
	// List returns the Draft's Commits, newest first, without their content.
	List(ctx context.Context, scope draft.Scope) ([]appdto.Commit, error)
	// Find returns one Commit; a Library's carries its WidgetTypes.
	Find(ctx context.Context, scope draft.Scope, number int) (appdto.Commit, error)
	// Rename changes the title of a Commit, stating no Draft Version.
	Rename(ctx context.Context, scope draft.Scope, number int, title string) (appdto.Commit, error)
	// ProjectFile returns the ProjectFile of a Project's Commit as a download.
	// It is refused with invalid input for a Library's scope.
	ProjectFile(ctx context.Context, scope draft.Scope, number int) (ProjectFileDownload, error)
}

type commitServiceImpl struct {
	content     ProjectContent
	libraries   LibraryContent
	builder     ProjectFileBuilder
	widgetTypes library.WidgetTypeRepositories
	releases    release.Repository
}

var _ CommitService = (*commitServiceImpl)(nil)

// NewCommitService returns the service of the Commits of the Projects aContent
// serves and of the Libraries aLibraries serves, assembling a Project's
// snapshot with aBuilder, reading a Library's Draft through someWidgetTypes and
// marking a Commit a Release published with someReleases.
func NewCommitService(aContent ProjectContent, aLibraries LibraryContent, aBuilder ProjectFileBuilder,
	someWidgetTypes library.WidgetTypeRepositories, someReleases release.Repository) CommitService {
	return &commitServiceImpl{content: aContent, libraries: aLibraries, builder: aBuilder,
		widgetTypes: someWidgetTypes, releases: someReleases}
}

func (s commitServiceImpl) Save(ctx context.Context, scope draft.Scope, rawVersion int, rawTitle, rawAuthor string) (appdto.Commit, error) {
	if !scope.IsProject() && !scope.IsLibrary() {
		return appdto.Commit{}, invalidInput(fmt.Errorf("scope: a project or a library is required"))
	}
	title, err := draft.NewCommitTitle(rawTitle)
	if err != nil {
		return appdto.Commit{}, invalidInput(err)
	}
	author, err := draft.NewAuthor(rawAuthor)
	if err != nil {
		return appdto.Commit{}, invalidInput(err)
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Commit{}, err
	}

	var content draft.CommitContent
	if scope.IsProject() {
		content, err = s.projectContent(ctx, scope, expected)
	} else {
		content, err = s.libraryContent(ctx, scope, expected)
	}
	if err != nil {
		return appdto.Commit{}, err
	}

	saved, err := s.content.Drafts().SaveCommit(ctx, scope, expected, title, author, content)
	if err != nil {
		return appdto.Commit{}, fmt.Errorf("cannot save %s: %w", scope, err)
	}
	return appdto.NewCommit(saved), nil
}

// projectContent checks the Project is live and at expected, then assembles
// its ProjectFile with the builder a Deploy uses. A refused build is a Draft
// that cannot be saved.
func (s commitServiceImpl) projectContent(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft]) (draft.CommitContent, error) {
	projectID, _, err := s.content.open(ctx, scope.ProjectID().UUID())
	if err != nil {
		return draft.CommitContent{}, err
	}
	current, err := s.content.DraftVersion(ctx, projectID)
	if err != nil {
		return draft.CommitContent{}, err
	}
	if current.Number() != expected.Number() {
		return draft.CommitContent{}, draft.ConflictError{Expected: expected.Number(), Current: current.Number(), Scope: scope}
	}
	file, err := s.builder.Build(ctx, projectID.UUID())
	if err != nil {
		if errors.Is(err, project.ErrProjectNotFound) {
			return draft.CommitContent{}, err
		}
		return draft.CommitContent{}, DraftNotSaveableError{Reason: err.Error()}
	}
	return draft.NewProjectCommitContent(file), nil
}

// libraryContent checks the Library is live and at expected, then takes the
// copies of its Draft's WidgetTypes.
func (s commitServiceImpl) libraryContent(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft]) (draft.CommitContent, error) {
	libraryID, err := s.libraries.open(ctx, scope.LibraryID().UUID())
	if err != nil {
		return draft.CommitContent{}, err
	}
	current, err := s.libraries.Version(ctx, libraryID.UUID())
	if err != nil {
		return draft.CommitContent{}, err
	}
	if current != expected.Number() {
		return draft.CommitContent{}, draft.ConflictError{Expected: expected.Number(), Current: current, Scope: scope}
	}
	types, err := s.widgetTypes.ForLibrary(libraryID).FindAll(ctx)
	if err != nil {
		return draft.CommitContent{}, fmt.Errorf("cannot read the draft of %s: %w", scope, err)
	}
	copies := make([]library.ReleasedWidgetType, len(types))
	for i, wt := range types {
		copies[i] = library.ReleasedFrom(wt)
	}
	return draft.NewLibraryCommitContent(copies), nil
}

func (s commitServiceImpl) List(ctx context.Context, scope draft.Scope) ([]appdto.Commit, error) {
	commits, err := s.content.Drafts().FindCommits(ctx, scope)
	if err != nil {
		return nil, err
	}
	dtos := appdto.NewCommitList(commits)
	if err := s.markReleased(ctx, scope, dtos); err != nil {
		return nil, err
	}
	return dtos, nil
}

func (s commitServiceImpl) Find(ctx context.Context, scope draft.Scope, rawNumber int) (appdto.Commit, error) {
	number, err := commitNumber(rawNumber)
	if err != nil {
		return appdto.Commit{}, err
	}
	commit, err := s.content.Drafts().FindCommit(ctx, scope, number)
	if err != nil {
		return appdto.Commit{}, err
	}
	dto := appdto.NewCommitWithContent(commit)
	return s.releasedCommit(ctx, scope, dto)
}

// releasedCommit returns dto with Released set when a Release published it.
func (s commitServiceImpl) releasedCommit(ctx context.Context, scope draft.Scope, dto appdto.Commit) (appdto.Commit, error) {
	dtos := []appdto.Commit{dto}
	if err := s.markReleased(ctx, scope, dtos); err != nil {
		return appdto.Commit{}, err
	}
	return dtos[0], nil
}

// markReleased sets Released on the Commits a Release published. Only a
// Project has Releases until task 72_5.
func (s commitServiceImpl) markReleased(ctx context.Context, scope draft.Scope, dtos []appdto.Commit) error {
	if !scope.IsProject() || len(dtos) == 0 {
		return nil
	}
	numbers, err := s.releases.ReleasedCommits(ctx, scope)
	if err != nil {
		return fmt.Errorf("cannot read the releases of %s: %w", scope, err)
	}
	released := make(map[int]struct{}, len(numbers))
	for _, number := range numbers {
		released[number] = struct{}{}
	}
	for i := range dtos {
		if _, ok := released[dtos[i].Number]; ok {
			dtos[i].Released = true
		}
	}
	return nil
}

func (s commitServiceImpl) Rename(ctx context.Context, scope draft.Scope, rawNumber int, rawTitle string) (appdto.Commit, error) {
	number, err := commitNumber(rawNumber)
	if err != nil {
		return appdto.Commit{}, err
	}
	title, err := draft.NewCommitTitle(rawTitle)
	if err != nil {
		return appdto.Commit{}, invalidInput(err)
	}
	commit, err := s.content.Drafts().RenameCommit(ctx, scope, number, title)
	if err != nil {
		return appdto.Commit{}, err
	}
	return appdto.NewCommit(commit), nil
}

func (s commitServiceImpl) ProjectFile(ctx context.Context, scope draft.Scope, rawNumber int) (ProjectFileDownload, error) {
	if !scope.IsProject() {
		return ProjectFileDownload{}, invalidInput(fmt.Errorf("commits of a library have no project file"))
	}
	number, err := commitNumber(rawNumber)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	projectID, _, err := s.content.open(ctx, scope.ProjectID().UUID())
	if err != nil {
		return ProjectFileDownload{}, err
	}
	commit, err := s.content.Drafts().FindCommit(ctx, scope, number)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	file, ok := commit.Content().ProjectFile()
	if !ok {
		return ProjectFileDownload{}, fmt.Errorf("commit %d of %s holds no project file", number, scope)
	}
	name, err := projectDownloadName(ctx, s.content, projectID)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	return ProjectFileDownload{File: file, ProjectName: name}, nil
}

// commitNumber parses the number of a Commit a client names; a number below 1
// is the client's input, not a missing Commit.
func commitNumber(raw int) (int, error) {
	if raw < 1 {
		return 0, invalidInput(fmt.Errorf("commit number must be positive, got %d", raw))
	}
	return raw, nil
}
