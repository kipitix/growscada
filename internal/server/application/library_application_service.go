package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// LibraryService is the service interface for working with the libraries of
// the server's Organization: the libraries themselves, their Releases and
// Discards (ADR 0007, ADR 0012). The WidgetTypes of a library's Draft have
// a service of their own, WidgetTypeService.
type LibraryService interface {
	FindAllLibraries(context.Context) ([]appdto.Library, error)
	FindLibraryByID(context.Context, uuid.UUID) (appdto.Library, error)
	FindLibraryByName(context.Context, string) (appdto.Library, error)
	CreateLibrary(context.Context, appdto.LibraryInput) (appdto.Library, error)
	UpdateLibrary(ctx context.Context, libraryID uuid.UUID, version int, input appdto.LibraryInput) (appdto.Library, error)
	DeleteLibraryByID(context.Context, uuid.UUID) (appdto.Library, error)

	// Release makes a Library Release of the library's Draft, expecting the
	// Version of that Draft the client saw.
	Release(ctx context.Context, libraryID uuid.UUID, version int, input appdto.ReleaseInput) (appdto.LibraryRelease, error)
	FindReleases(ctx context.Context, libraryID uuid.UUID) ([]appdto.LibraryRelease, error)
	FindRelease(ctx context.Context, libraryID uuid.UUID, number int) (appdto.LibraryRelease, error)
	// DraftState is the Version of the library's Draft: what every write of
	// its content states and every DTO of it carries.
	DraftState(ctx context.Context, libraryID uuid.UUID) (appdto.DraftState, error)
	// Discard resets the library's Draft to its latest release in one
	// transaction over the Draft and answers the Draft's new state.
	Discard(ctx context.Context, libraryID uuid.UUID) (appdto.DraftState, error)
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
// of NewLibraryCleanupPolicy. A Release is gated by the Draft's Version and
// clears its DraftChange stack once made; a Discard is one transaction over
// the Draft (ADR 0016, ADR 0017). The Runtime node aRuntime is asked whether a
// running Deployment names a Release of a library about to be deleted
// (task 71).
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

// describe returns the DTO of l with its latest release and whether its
// Draft has changes not released yet.
func (s libraryServiceImpl) describe(ctx context.Context, l library.Library) (appdto.Library, error) {
	list, err := s.describeAll(ctx, []library.Library{l})
	if err != nil {
		return appdto.Library{}, err
	}
	return list[0], nil
}

// describeAll is describe of every library of list: its Draft compared with
// its latest release by library.ReleaseStateOf, the one function that also
// decides whether a Release is refused.
func (s libraryServiceImpl) describeAll(ctx context.Context, list []library.Library) ([]appdto.Library, error) {
	result := make([]appdto.Library, len(list))
	for i, l := range list {
		latest, err := s.releases.FindLatest(ctx, l.ID())
		if err != nil {
			return nil, fmt.Errorf("cannot find the latest release of library %s: %w", l.ID(), err)
		}
		draft, err := s.widgetTypes.ForLibrary(l.ID()).FindAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot find the draft of library %s: %w", l.ID(), err)
		}
		result[i] = appdto.NewLibrary(l, library.ReleaseStateOf(draft, latest))
	}
	return result, nil
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
	saved, err := s.libraries.Save(ctx, library.CreateLibrary(s.libraries.NextID(), s.organization, name))
	if err != nil {
		return appdto.Library{}, fmt.Errorf("cannot save library: %w", err)
	}
	return appdto.NewLibrary(saved, library.ReleaseState{DraftDiffers: true}), nil
}

// UpdateLibrary renames a library, expecting its Version.
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
	renamed, err := found.Rename(expected, name)
	if err != nil {
		return appdto.Library{}, fmt.Errorf("cannot update library: %w", err)
	}
	saved, err := s.libraries.Save(ctx, renamed)
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

// libraryDraftVersion parses the Version of a Library's Draft a client
// states; a negative one is no version at all: the client's input, not a
// conflict.
func libraryDraftVersion(raw int) (version.Version[library.DraftContent], error) {
	expected, err := version.New(version.WithNumber[library.DraftContent](raw))
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

// Release makes the next Library Release of the library from its Draft as
// it is now. It is refused if the Draft is not at the Version the client
// states, and if it has not changed since the latest release. A Release that
// loses the race for its number to another is made again on top of it, the
// Draft reread. The DraftChange stack is cleared after the release rows are
// committed: a failure to clear is reported while the release stands
// (ADR 0017).
func (s libraryServiceImpl) Release(ctx context.Context, rawID uuid.UUID, rawExpectedVersion int, input appdto.ReleaseInput) (appdto.LibraryRelease, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.LibraryRelease{}, err
	}
	expected, err := libraryDraftVersion(rawExpectedVersion)
	if err != nil {
		return appdto.LibraryRelease{}, err
	}

	var saved library.LibraryRelease
	err = retryOnRace(library.ErrReleaseNumberTaken, func() error {
		latest, err := s.releases.FindLatest(ctx, found.ID())
		if err != nil {
			return fmt.Errorf("cannot find latest release: %w", err)
		}
		draft, err := s.widgetTypes.ForLibrary(found.ID()).FindAll(ctx)
		if err != nil {
			return fmt.Errorf("cannot read draft: %w", err)
		}
		release, err := library.CreateRelease(found, draft, latest, input.Note, s.now())
		if err != nil {
			return fmt.Errorf("cannot release library: %w", err)
		}
		if saved, err = s.releases.Save(ctx, release, expected); err != nil {
			return fmt.Errorf("cannot save release: %w", err)
		}
		return nil
	})
	if err != nil {
		return appdto.LibraryRelease{}, err
	}
	if err := s.content.Drafts().ClearStack(ctx, s.content.ScopeOf(found.ID())); err != nil {
		return appdto.LibraryRelease{}, fmt.Errorf("cannot clear the draft stack of released library %s: %w", found.ID(), err)
	}
	return appdto.NewLibraryRelease(saved), nil
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
	return appdto.NewLibraryReleaseList(list), nil
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
	return appdto.NewLibraryRelease(release), nil
}

// DraftState returns the Version of the library's Draft.
func (s libraryServiceImpl) DraftState(ctx context.Context, rawID uuid.UUID) (appdto.DraftState, error) {
	found, err := s.findOwn(ctx, rawID)
	if err != nil {
		return appdto.DraftState{}, err
	}
	version, err := s.content.Version(ctx, found.ID().UUID())
	if err != nil {
		return appdto.DraftState{}, err
	}
	return appdto.DraftState{Version: version}, nil
}

// Discard resets the Draft of the library to its latest release in one
// transaction over the Draft: the WidgetTypes added since go, the changed ones
// are restored and the removed ones brought back, and the DraftChange stack
// is cleared. A library never released has nothing to go back to.
func (s libraryServiceImpl) Discard(ctx context.Context, rawID uuid.UUID) (appdto.DraftState, error) {
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
	result, err := s.content.Drafts().Replace(ctx, s.content.ScopeOf(found.ID()), func(session *draft.Session) error {
		return session.ReplaceWidgetTypes(ctx, latest)
	})
	if err != nil {
		return appdto.DraftState{}, fmt.Errorf("cannot discard library %s: %w", found.ID(), err)
	}
	return appdto.DraftState{Version: result.Version().Number()}, nil
}
