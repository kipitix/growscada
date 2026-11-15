package application

import (
	"context"
	"fmt"

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
// the server's Organization: the libraries themselves, their Drafts and their
// saved states (ADR 0007, ADR 0012, ADR 0021). The WidgetTypes of a library's
// Draft have a service of their own, WidgetTypeService.
type LibraryService interface {
	FindAllLibraries(context.Context) ([]appdto.Library, error)
	FindLibraryByID(context.Context, uuid.UUID) (appdto.Library, error)
	FindLibraryByName(context.Context, string) (appdto.Library, error)
	CreateLibrary(context.Context, appdto.LibraryInput) (appdto.Library, error)
	UpdateLibrary(ctx context.Context, libraryID uuid.UUID, version int, input appdto.LibraryInput) (appdto.Library, error)
	DeleteLibraryByID(context.Context, uuid.UUID) (appdto.Library, error)

	// DraftState is the Version of the library's Draft: what every write of
	// its content states and every DTO of it carries.
	DraftState(ctx context.Context, libraryID uuid.UUID) (appdto.DraftState, error)
	// Discard resets the library's Draft to its last saved state in one transaction
	// over the Draft and answers the Draft's new state. A library never saved
	// is ErrNothingToDiscard.
	Discard(ctx context.Context, libraryID uuid.UUID) (appdto.DraftState, error)
	// LoadCommit replaces the library's Draft with the WidgetTypes of its
	// saved state number, in one transaction: the saved state becomes what is
	// edited, the DraftChange stack is cleared and the Version raised, as a
	// Discard does. The Draft is clean exactly when that saved state is its last one
	// (task 84). A number the library does not have is draft.ErrCommitNotFound.
	LoadCommit(ctx context.Context, libraryID uuid.UUID, number int) (appdto.LoadedDraft, error)
}

type libraryServiceImpl struct {
	content      LibraryContent
	libraries    library.LibraryRepository
	widgetTypes  library.WidgetTypeRepositories
	runtime      RuntimeNode
	organization id.ID[organization.Organization]
}

var _ LibraryService = (*libraryServiceImpl)(nil)

// NewLibraryService returns the service of the libraries of the Organization
// aContent serves. The events of its changes are recorded by the aggregates
// and saved with them (ADR 0008); a deleted library is removed by the policy
// of NewLibraryCleanupPolicy. A Discard is one transaction over the Draft
// (ADR 0016, ADR 0017, ADR 0021). The Runtime node aRuntime is asked whether a
// running Deployment names a saved state of a library about to be deleted
// (task 71).
func NewLibraryService(
	aContent LibraryContent,
	someLibraries library.LibraryRepository,
	someWidgetTypes library.WidgetTypeRepositories,
	aRuntime RuntimeNode,
) LibraryService {
	return &libraryServiceImpl{
		content:      aContent,
		libraries:    someLibraries,
		widgetTypes:  someWidgetTypes,
		runtime:      aRuntime,
		organization: aContent.organization,
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

// describe returns the DTO of l with its last saved state and whether its Draft has
// changes not saved yet.
func (s libraryServiceImpl) describe(ctx context.Context, l library.Library) (appdto.Library, error) {
	list, err := s.describeAll(ctx, []library.Library{l})
	if err != nil {
		return appdto.Library{}, err
	}
	return list[0], nil
}

// describeAll is describe of every library of list: its last saved state and
// whether its Draft moved since it (task 74).
func (s libraryServiceImpl) describeAll(ctx context.Context, list []library.Library) ([]appdto.Library, error) {
	result := make([]appdto.Library, len(list))
	for i, l := range list {
		lastCommit, found, err := s.content.Drafts().FindLastCommit(ctx, s.content.ScopeOf(l.ID()))
		if err != nil {
			return nil, fmt.Errorf("cannot find the last saved state of library %s: %w", l.ID(), err)
		}
		lastCommitNumber := 0
		if found {
			lastCommitNumber = lastCommit.Number()
		}
		unsaved, err := s.unsavedChanges(ctx, l.ID())
		if err != nil {
			return nil, err
		}
		result[i] = appdto.NewLibrary(l, lastCommitNumber, unsaved)
	}
	return result, nil
}

// unsavedChanges reports whether the library's Draft moved since its content
// last equalled its last saved state: the Draft's own flag, not a comparison of
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
	// A new Draft has no saved state and no changes: it is clean and never saved.
	return appdto.NewLibrary(saved, 0, false), nil
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

// DeleteLibraryByID marks a library deleted, while no live Project imports it and
// no running Deployment names a saved state of it, and returns it as it was read,
// with its releases: from then on it does not exist for the API. Its Draft and
// releases are removed later, by the policy of NewLibraryCleanupPolicy. A
// saved state of the past does not refuse it: their documents
// carry the WidgetType copies they need (task 71). It expects no Version
// (retryOnRace).
func (s libraryServiceImpl) DeleteLibraryByID(ctx context.Context, rawID uuid.UUID) (appdto.Library, error) {
	var deleted appdto.Library
	err := retryOnRace(library.ErrLibraryConflict, func() error {
		found, err := s.findOwn(ctx, rawID)
		if err != nil {
			return err
		}
		// A running Deployment naming one of the library's saved states would make
		// a Discard of it unable to restore the import; the imports alone do not tell
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

// DraftState returns the Version of the library's Draft and whether it holds
// changes its last saved state does not.
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
		// first write states. It has no content to differ from a saved state.
		return appdto.DraftState{Version: 1}, nil
	}
	return appdto.DraftState{Version: read.Version().Number(), HasUnsavedChanges: read.HasUnsavedChanges()}, nil
}

// Discard resets the Draft of the library to its last saved state in one
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
		return appdto.DraftState{}, fmt.Errorf("cannot read the last saved state of library %s: %w", found.ID(), err)
	}
	if !ok {
		return appdto.DraftState{}, fmt.Errorf("%w: %s", library.ErrNothingToDiscard, found.ID())
	}
	types, ok := last.Content().WidgetTypes()
	if !ok {
		return appdto.DraftState{}, fmt.Errorf("saved state %d of library %s holds no widget types", last.Number(), found.ID())
	}
	result, err := s.content.Drafts().Replace(ctx, scope, last.Number(), func(session *draft.Session) error {
		return session.ReplaceWidgetTypes(ctx, types)
	})
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot discard library %s: %w", found.ID(), err)
	}
	return appdto.DraftState{Version: result.Version().Number(), HasUnsavedChanges: result.HasUnsavedChanges()}, nil
}

// LoadCommit makes the library's Draft the content of one of its saved states
// (task 84): the reset a Discard makes, to the saved state the client named rather
// than the last one.
func (s libraryServiceImpl) LoadCommit(ctx context.Context, rawID uuid.UUID, number int) (appdto.LoadedDraft, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.LoadedDraft{}, err
	}
	scope := s.content.ScopeOf(found.ID())
	commit, err := s.content.Drafts().FindCommit(ctx, scope, number)
	if err != nil {
		return appdto.LoadedDraft{}, fmt.Errorf("cannot read saved state %d of library %s: %w", number, found.ID(), err)
	}
	types, ok := commit.Content().WidgetTypes()
	if !ok {
		return appdto.LoadedDraft{}, fmt.Errorf("saved state %d of library %s holds no widget types", commit.Number(), found.ID())
	}
	// The loaded content is the Draft's clean state only when it is the last
	// saved state: an older one leaves the Draft with unsaved changes (task 74).
	// The comparison is the repository's, under the Draft's lock.
	result, err := s.content.Drafts().Replace(ctx, scope, commit.Number(),
		func(session *draft.Session) error {
			return session.ReplaceWidgetTypes(ctx, types)
		})
	if err != nil {
		return appdto.LoadedDraft{}, fmt.Errorf("cannot load saved state %d of library %s: %w", commit.Number(), found.ID(), err)
	}
	return appdto.LoadedDraft{
		Version:           result.Version().Number(),
		HasUnsavedChanges: result.HasUnsavedChanges(),
	}, nil
}
