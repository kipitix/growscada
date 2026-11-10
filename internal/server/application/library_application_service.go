package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/description"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// LibraryService is the service interface for working with the libraries of
// the server's Organization: the libraries themselves, their Releases,
// Discards and Reverts (ADR 0007, ADR 0012, ADR 0021). The WidgetTypes of a
// library's Draft have a service of their own, WidgetTypeService.
type LibraryService interface {
	FindAllLibraries(context.Context) ([]appdto.Library, error)
	FindLibraryByID(context.Context, uuid.UUID) (appdto.Library, error)
	FindLibraryByName(context.Context, string) (appdto.Library, error)
	CreateLibrary(context.Context, appdto.LibraryInput) (appdto.Library, error)
	UpdateLibrary(ctx context.Context, libraryID uuid.UUID, version int, input appdto.LibraryInput) (appdto.Library, error)
	DeleteLibraryByID(context.Context, uuid.UUID) (appdto.Library, error)

	// Release publishes a Commit of the library's Draft as the next Library
	// Release. It reads no Draft Version: a Commit is immutable. A Commit is
	// published at most once (ErrCommitAlreadyReleased); one the library does
	// not have is draft.ErrCommitNotFound.
	Release(ctx context.Context, libraryID uuid.UUID, input appdto.ReleaseInput) (appdto.LibraryRelease, error)
	FindReleases(ctx context.Context, libraryID uuid.UUID) ([]appdto.LibraryRelease, error)
	FindRelease(ctx context.Context, libraryID uuid.UUID, number int) (appdto.LibraryRelease, error)
	// UpdateReleaseNote changes the note of a Release. It is metadata: no
	// Draft Version is stated and the content never changes.
	UpdateReleaseNote(ctx context.Context, libraryID uuid.UUID, number int, note string) (appdto.LibraryRelease, error)
	// DraftState is the Version of the library's Draft: what every write of
	// its content states and every DTO of it carries.
	DraftState(ctx context.Context, libraryID uuid.UUID) (appdto.DraftState, error)
	// Discard resets the library's Draft to its last Commit in one transaction
	// over the Draft and answers the Draft's new state. A library never saved
	// is ErrNothingToDiscard.
	Discard(ctx context.Context, libraryID uuid.UUID) (appdto.DraftState, error)
	// RevertToLatestRelease resets the library's Draft to its latest Release
	// in one transaction over the Draft and answers the Draft's new state. A
	// library never released is ErrNoRelease.
	RevertToLatestRelease(ctx context.Context, libraryID uuid.UUID) (appdto.DraftState, error)
}

type libraryServiceImpl struct {
	content      LibraryContent
	libraries    library.LibraryRepository
	releases     library.LibraryReleaseRepository
	widgetTypes  library.WidgetTypeRepositories
	runtime      RuntimeNode
	organization id.ID[organization.Organization]
	now          func() time.Time
}

var _ LibraryService = (*libraryServiceImpl)(nil)

// NewLibraryService returns the service of the libraries of the Organization
// aContent serves. The events of its changes are recorded by the aggregates
// and saved with them (ADR 0008); a deleted library is removed by the policy
// of NewLibraryCleanupPolicy. A Release publishes a Commit and changes no
// Draft; a Discard and a Revert are one transaction over the Draft (ADR 0016,
// ADR 0017, ADR 0021). The Runtime node aRuntime is asked whether a running
// Deployment names a Release of a library about to be deleted (task 71).
func NewLibraryService(
	aContent LibraryContent,
	someLibraries library.LibraryRepository,
	someReleases library.LibraryReleaseRepository,
	someWidgetTypes library.WidgetTypeRepositories,
	aRuntime RuntimeNode,
) LibraryService {
	return &libraryServiceImpl{
		content:      aContent,
		libraries:    someLibraries,
		releases:     someReleases,
		widgetTypes:  someWidgetTypes,
		runtime:      aRuntime,
		organization: aContent.organization,
		now:          time.Now,
	}
}

// findOwnLibrary returns the live Library libraryID of the Organization
// orgID; one of another Organization is not found.
func findOwnLibrary(ctx context.Context, libraries library.LibraryRepository, orgID id.ID[organization.Organization], libraryID id.ID[library.Library]) (library.Library, error) {
	found, err := libraries.FindByID(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("error on find library by id in repository: %w", err)
	}
	if found.OrganizationID() != orgID {
		return nil, id.NotFound(library.ErrLibraryNotFound, libraryID)
	}
	return found, nil
}

func (s libraryServiceImpl) findOwn(ctx context.Context, rawID uuid.UUID) (library.Library, error) {
	return findOwnLibrary(ctx, s.libraries, s.organization, id.NewID(id.IDWithUUID[library.Library](rawID)))
}

// describe returns the DTO of l with its latest release, its last Commit and
// whether its Draft has changes not saved or Commits not released yet.
func (s libraryServiceImpl) describe(ctx context.Context, l library.Library) (appdto.Library, error) {
	list, err := s.describeAll(ctx, []library.Library{l})
	if err != nil {
		return appdto.Library{}, err
	}
	return list[0], nil
}

// describeAll is describe of every library of list: its latest release, its
// last Commit and the Version of its Draft, by library.ReleaseStateOf, the one
// function that decides what is saved and what is published.
func (s libraryServiceImpl) describeAll(ctx context.Context, list []library.Library) ([]appdto.Library, error) {
	result := make([]appdto.Library, len(list))
	for i, l := range list {
		state, err := s.releaseState(ctx, l)
		if err != nil {
			return nil, err
		}
		result[i] = appdto.NewLibrary(l, state)
	}
	return result, nil
}

// releaseState reads what a library's Commits, Releases and Draft tell of it.
func (s libraryServiceImpl) releaseState(ctx context.Context, l library.Library) (library.ReleaseState, error) {
	latest, err := s.releases.FindLatest(ctx, l.ID())
	if err != nil {
		return library.ReleaseState{}, fmt.Errorf("cannot find the latest release of library %s: %w", l.ID(), err)
	}
	lastCommit, found, err := s.content.Drafts().FindLastCommit(ctx, s.content.ScopeOf(l.ID()))
	if err != nil {
		return library.ReleaseState{}, fmt.Errorf("cannot find the last commit of library %s: %w", l.ID(), err)
	}
	lastCommitNumber := 0
	if found {
		lastCommitNumber = lastCommit.Number()
	}
	unsaved, err := s.unsavedChanges(ctx, l.ID())
	if err != nil {
		return library.ReleaseState{}, err
	}
	return library.ReleaseStateOf(latest, lastCommitNumber, unsaved), nil
}

// unsavedChanges reports whether the library's Draft moved since its content
// last equalled its last Commit: the Draft's own flag, not a comparison of
// numbers kept here (ADR 0016, amendment 2026-11-10). A Draft never written —
// no row — is clean.
func (s libraryServiceImpl) unsavedChanges(ctx context.Context, libraryID id.ID[library.Library]) (bool, error) {
	found, ok, err := s.content.Drafts().Read(ctx, s.content.ScopeOf(libraryID))
	if err != nil {
		return false, fmt.Errorf("cannot read the draft of library %s: %w", libraryID, err)
	}
	if !ok {
		return false, nil
	}
	return found.HasUnsavedChanges(), nil
}

func (s libraryServiceImpl) FindAllLibraries(ctx context.Context) ([]appdto.Library, error) {
	list, err := s.libraries.FindAll(ctx, s.organization)
	if err != nil {
		return nil, fmt.Errorf("error on find libraries in repository: %w", err)
	}
	return s.describeAll(ctx, list)
}

func (s libraryServiceImpl) FindLibraryByID(ctx context.Context, rawID uuid.UUID) (appdto.Library, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.Library{}, err
	}
	return s.describe(ctx, found)
}

// FindLibraryByName returns the live library with the name, unique in the
// Organization.
func (s libraryServiceImpl) FindLibraryByName(ctx context.Context, rawName string) (appdto.Library, error) {
	name, err := library.NewLibraryName(rawName)
	if err != nil {
		return appdto.Library{}, fmt.Errorf("cannot find library because of name: %w", invalidInput(err))
	}
	found, err := s.libraries.FindByName(ctx, s.organization, name)
	if err != nil {
		return appdto.Library{}, fmt.Errorf("error on find library by name in repository: %w", err)
	}
	return s.describe(ctx, found)
}

func (s libraryServiceImpl) CreateLibrary(ctx context.Context, input appdto.LibraryInput) (appdto.Library, error) {
	name, err := library.NewLibraryName(input.Name)
	if err != nil {
		return appdto.Library{}, fmt.Errorf("cannot create library: name: %w", invalidInput(err))
	}
	desc, err := descriptionOf(input.Description, description.Description{})
	if err != nil {
		return appdto.Library{}, fmt.Errorf("cannot create library: %w", err)
	}
	saved, err := s.libraries.Save(ctx, library.CreateLibrary(s.libraries.NextID(), s.organization, name, desc))
	if err != nil {
		return appdto.Library{}, fmt.Errorf("cannot save library: %w", err)
	}
	// A new Draft has no Commit and no changes: it is clean and never saved.
	return appdto.NewLibrary(saved, library.ReleaseState{}), nil
}

// UpdateLibrary changes a library's name and description, expecting its
// Version. A nil description keeps the current one (task 73).
func (s libraryServiceImpl) UpdateLibrary(ctx context.Context, rawID uuid.UUID, rawExpectedVersion int, input appdto.LibraryInput) (appdto.Library, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.Library{}, err
	}
	expected, err := libraryVersion(rawExpectedVersion)
	if err != nil {
		return appdto.Library{}, err
	}
	// A stale write is a conflict whatever it sends.
	if err := found.CheckVersion(expected); err != nil {
		return appdto.Library{}, fmt.Errorf("cannot update library: %w", err)
	}
	name, err := library.NewLibraryName(input.Name)
	if err != nil {
		return appdto.Library{}, fmt.Errorf("cannot update library: name: %w", invalidInput(err))
	}
	desc, err := descriptionOf(input.Description, found.Description())
	if err != nil {
		return appdto.Library{}, fmt.Errorf("cannot update library: %w", err)
	}
	updated, err := found.Update(expected, name, desc)
	if err != nil {
		return appdto.Library{}, fmt.Errorf("cannot update library: %w", err)
	}
	saved, err := s.libraries.Save(ctx, updated)
	if err != nil {
		return appdto.Library{}, fmt.Errorf("cannot save library: %w", err)
	}
	return s.describe(ctx, saved)
}

// libraryVersion parses the Version a client states; a negative one is no
// version at all: the client's input, not a conflict.
func libraryVersion(raw int) (version.Version[library.Library], error) {
	expected, err := version.New(version.WithNumber[library.Library](raw))
	if err != nil {
		return expected, invalidInput(fmt.Errorf("version: %w", err))
	}
	return expected, nil
}

// DeleteLibraryByID marks a library deleted, while no live Project pins it and
// no running Deployment names a Release of it, and returns it as it was read,
// with its releases: from then on it does not exist for the API. Its Draft and
// releases are removed later, by the policy of NewLibraryCleanupPolicy. A
// Commit or a Project Release of the past does not refuse it: their documents
// carry the WidgetType copies they need (task 71). It expects no Version
// (retryOnRace).
func (s libraryServiceImpl) DeleteLibraryByID(ctx context.Context, rawID uuid.UUID) (appdto.Library, error) {
	var deleted appdto.Library
	err := retryOnRace(library.ErrLibraryConflict, func() error {
		found, err := s.findOwn(ctx, rawID)
		if err != nil {
			return err
		}
		// A running Deployment naming one of the library's Releases would make
		// a Discard of it unable to restore the pin; the pins alone do not tell
		// (task 71).
		named, err := s.runtime.DeploymentNamesLibrary(ctx, found.ID().UUID())
		if err != nil {
			return fmt.Errorf("cannot read the deployments naming library %s: %w", found.ID(), err)
		}
		if named {
			return fmt.Errorf("%w: %s", ErrLibraryInRunningDeployment, found.ID())
		}
		if deleted, err = s.describe(ctx, found); err != nil {
			return err
		}
		if _, err := s.libraries.Save(ctx, found.Delete()); err != nil {
			return fmt.Errorf("cannot delete library: %w", err)
		}
		return nil
	})
	if err != nil {
		return appdto.Library{}, err
	}
	return deleted, nil
}

// Release publishes a Commit of the library's Draft as the next Library
// Release, with the note and the Author. It reads no Draft Version: a Commit
// is immutable, and its content is the snapshot the Release copies. A Release
// that loses the race for its number to another is made again on top of it.
// The DraftChange stack is untouched: a Release changes no Draft (ADR 0021).
func (s libraryServiceImpl) Release(ctx context.Context, rawID uuid.UUID, input appdto.ReleaseInput) (appdto.LibraryRelease, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.LibraryRelease{}, err
	}
	number, err := commitNumber(input.Commit)
	if err != nil {
		return appdto.LibraryRelease{}, err
	}
	author, err := library.NewAuthor(input.Author)
	if err != nil {
		return appdto.LibraryRelease{}, invalidInput(err)
	}
	scope := s.content.ScopeOf(found.ID())
	commit, err := s.content.Drafts().FindCommit(ctx, scope, number)
	if err != nil {
		return appdto.LibraryRelease{}, err
	}
	types, ok := commit.Content().WidgetTypes()
	if !ok {
		return appdto.LibraryRelease{}, fmt.Errorf("commit %d of library %s holds no widget types", number, found.ID())
	}

	var saved library.LibraryRelease
	err = retryOnRace(library.ErrReleaseNumberTaken, func() error {
		latest, err := s.releases.FindLatest(ctx, found.ID())
		if err != nil {
			return fmt.Errorf("cannot find latest release: %w", err)
		}
		releaseNumber := library.FirstRelease()
		if latest != nil {
			if latest.LibraryID() != found.ID() {
				return fmt.Errorf("cannot release library %s on top of a release of library %s", found.ID(), latest.LibraryID())
			}
			releaseNumber = latest.Number().Next()
		}
		release, err := library.CreateRelease(found, releaseNumber, commit.Number(), input.Note, author, s.now(), types)
		if err != nil {
			return fmt.Errorf("cannot release library: %w", err)
		}
		if saved, err = s.releases.Save(ctx, release); err != nil {
			return fmt.Errorf("cannot save release: %w", err)
		}
		return nil
	})
	if err != nil {
		return appdto.LibraryRelease{}, err
	}
	return appdto.NewLibraryRelease(saved, commit.Title().String()), nil
}

func (s libraryServiceImpl) FindReleases(ctx context.Context, rawID uuid.UUID) ([]appdto.LibraryRelease, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return nil, err
	}
	list, err := s.releases.FindAll(ctx, found.ID())
	if err != nil {
		return nil, fmt.Errorf("error on find releases in repository: %w", err)
	}
	titles, err := s.commitTitles(ctx, found)
	if err != nil {
		return nil, err
	}
	return appdto.NewLibraryReleaseList(list, titles), nil
}

func (s libraryServiceImpl) FindRelease(ctx context.Context, rawID uuid.UUID, rawNumber int) (appdto.LibraryRelease, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.LibraryRelease{}, err
	}
	number, err := library.NewReleaseNumber(rawNumber)
	if err != nil {
		// No release has such a number.
		return appdto.LibraryRelease{}, fmt.Errorf("%w: %d of library %s", library.ErrReleaseNotFound, rawNumber, found.ID())
	}
	release, err := s.releases.FindByNumber(ctx, found.ID(), number)
	if err != nil {
		return appdto.LibraryRelease{}, fmt.Errorf("error on find release in repository: %w", err)
	}
	title, err := publishedCommitTitle(ctx, s.content.Drafts(), found.ID(), release)
	if err != nil {
		return appdto.LibraryRelease{}, err
	}
	return appdto.NewLibraryRelease(release, title), nil
}

// UpdateReleaseNote changes the note of a Release. The content never changes
// and no Draft Version is stated.
func (s libraryServiceImpl) UpdateReleaseNote(ctx context.Context, rawID uuid.UUID, rawNumber int, note string) (appdto.LibraryRelease, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.LibraryRelease{}, err
	}
	number, err := library.NewReleaseNumber(rawNumber)
	if err != nil {
		return appdto.LibraryRelease{}, fmt.Errorf("%w: %d of library %s", library.ErrReleaseNotFound, rawNumber, found.ID())
	}
	updated, err := s.releases.UpdateNote(ctx, found.ID(), number, note)
	if err != nil {
		return appdto.LibraryRelease{}, fmt.Errorf("error on update release note: %w", err)
	}
	title, err := publishedCommitTitle(ctx, s.content.Drafts(), found.ID(), updated)
	if err != nil {
		return appdto.LibraryRelease{}, err
	}
	return appdto.NewLibraryRelease(updated, title), nil
}

// commitTitles returns the current title of every Commit of the library,
// keyed by number: a Release shows the title of the Commit it published.
func (s libraryServiceImpl) commitTitles(ctx context.Context, l library.Library) (map[int]string, error) {
	commits, err := s.content.Drafts().FindCommits(ctx, s.content.ScopeOf(l.ID()))
	if err != nil {
		return nil, fmt.Errorf("cannot read the commits of library %s: %w", l.ID(), err)
	}
	titles := make(map[int]string, len(commits))
	for _, commit := range commits {
		titles[commit.Number()] = commit.Title().String()
	}
	return titles, nil
}

// publishedCommitTitle returns the current title of the Commit a Library
// Release published; empty for a legacy Release and for one whose Commit this
// server does not have.
func publishedCommitTitle(ctx context.Context, drafts draft.Repository, libraryID id.ID[library.Library], release library.LibraryRelease) (string, error) {
	number := release.CommitNumber()
	if number == nil {
		return "", nil
	}
	commit, err := drafts.FindCommit(ctx, draft.LibraryScope(libraryID), *number)
	if errors.Is(err, draft.ErrCommitNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cannot read commit %d of library %s: %w", *number, libraryID, err)
	}
	return commit.Title().String(), nil
}

// DraftState returns the Version of the library's Draft and whether it holds
// changes its last Commit does not.
func (s libraryServiceImpl) DraftState(ctx context.Context, rawID uuid.UUID) (appdto.DraftState, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.DraftState{}, err
	}
	read, ok, err := s.content.Drafts().Read(ctx, s.content.ScopeOf(found.ID()))
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot read the draft of library %s: %w", found.ID(), err)
	}
	if !ok {
		// A Draft never written is a fresh one at version 1: the Version its
		// first write states. It has no content to differ from a Commit.
		return appdto.DraftState{Version: 1}, nil
	}
	return appdto.DraftState{Version: read.Version().Number(), HasUnsavedChanges: read.HasUnsavedChanges()}, nil
}

// Discard resets the Draft of the library to its last Commit in one
// transaction over the Draft: the WidgetTypes added or changed since go, the
// removed ones are brought back, and the DraftChange stack is cleared. A
// library never saved has nothing to go back to (ErrNothingToDiscard).
func (s libraryServiceImpl) Discard(ctx context.Context, rawID uuid.UUID) (appdto.DraftState, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.DraftState{}, err
	}
	scope := s.content.ScopeOf(found.ID())
	last, ok, err := s.content.Drafts().FindLastCommit(ctx, scope)
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot read the last commit of library %s: %w", found.ID(), err)
	}
	if !ok {
		return appdto.DraftState{}, fmt.Errorf("%w: %s", library.ErrNothingToDiscard, found.ID())
	}
	types, ok := last.Content().WidgetTypes()
	if !ok {
		return appdto.DraftState{}, fmt.Errorf("commit %d of library %s holds no widget types", last.Number(), found.ID())
	}
	result, err := s.content.Drafts().Replace(ctx, scope, true, func(session *draft.Session) error {
		return session.ReplaceWidgetTypes(ctx, types)
	})
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot discard library %s: %w", found.ID(), err)
	}
	return appdto.DraftState{Version: result.Version().Number(), HasUnsavedChanges: result.HasUnsavedChanges()}, nil
}

// RevertToLatestRelease resets the Draft of the library to its latest Release
// in one transaction over the Draft and clears the DraftChange stack. A
// library never released has nothing to go back to (ErrNoRelease).
func (s libraryServiceImpl) RevertToLatestRelease(ctx context.Context, rawID uuid.UUID) (appdto.DraftState, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.DraftState{}, err
	}
	latest, err := s.releases.FindLatest(ctx, found.ID())
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot find latest release: %w", err)
	}
	if latest == nil {
		return appdto.DraftState{}, fmt.Errorf("%w: %s", library.ErrNoRelease, found.ID())
	}
	// The Draft the Revert writes is its last Commit's content only when the
	// latest Release published that Commit: then it is clean, as after a
	// Discard. An older Release is a different content, and the marker stays
	// where it was (task 74).
	clean, err := s.revertIsTheLastCommit(ctx, found.ID(), latest)
	if err != nil {
		return appdto.DraftState{}, err
	}
	result, err := s.content.Drafts().Replace(ctx, s.content.ScopeOf(found.ID()), clean, func(session *draft.Session) error {
		return session.ReplaceWidgetTypes(ctx, latest.WidgetTypes())
	})
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot revert library %s: %w", found.ID(), err)
	}
	return appdto.DraftState{Version: result.Version().Number(), HasUnsavedChanges: result.HasUnsavedChanges()}, nil
}

// revertIsTheLastCommit reports whether the latest Release published the last
// Commit of the library's Draft: the WidgetType copies of the two are the same.
// A Release before Commits, or one of an older Commit, is not it.
func (s libraryServiceImpl) revertIsTheLastCommit(ctx context.Context, libraryID id.ID[library.Library], latest library.LibraryRelease) (bool, error) {
	last, found, err := s.content.Drafts().FindLastCommit(ctx, s.content.ScopeOf(libraryID))
	if err != nil {
		return false, fmt.Errorf("cannot read the last commit of library %s: %w", libraryID, err)
	}
	if !found {
		return false, nil
	}
	types, ok := last.Content().WidgetTypes()
	if !ok {
		return false, nil
	}
	return sameWidgetTypeCopies(latest.WidgetTypes(), types), nil
}

// sameWidgetTypeCopies reports whether two lists of released copies hold the
// same WidgetTypes with the same definitions, in the same order: what a Revert
// to a Release compares before it can call the Draft clean (task 74).
func sameWidgetTypeCopies(a, b []library.ReleasedWidgetType) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].SameAs(b[i]) {
			return false
		}
	}
	return true
}
