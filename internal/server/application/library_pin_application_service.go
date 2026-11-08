package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// LibraryPinService is the service interface for working with the Library
// Pins of a project's Draft and, through them, the WidgetTypes its Widgets
// may be of (ADR 0007, ADR 0012). Every write states the Version of the Draft
// (ADR 0016). Every method takes the project first; a project that does not
// exist (or is deleted) is an error wrapping project.ErrProjectNotFound, a
// library that does not exist one wrapping library.ErrLibraryNotFound.
type LibraryPinService interface {
	FindLibraryPins(ctx context.Context, projectID uuid.UUID) ([]appdto.LibraryPin, error)
	// PutLibraryPin pins the release of the library: it adds the pin when the
	// project pins no release of the library, else switches it, stating the
	// Version of the Draft. The bool reports whether the pin was added.
	PutLibraryPin(ctx context.Context, projectID, libraryID uuid.UUID, release, version int) (appdto.LibraryPin, bool, error)
	DeleteLibraryPin(ctx context.Context, projectID, libraryID uuid.UUID, version int) (appdto.LibraryPin, error)
	// PinImpact previews switching the pin of the library to release, or
	// removing it for nil.
	PinImpact(ctx context.Context, projectID, libraryID uuid.UUID, release *int) (appdto.PinImpact, error)
	// FindPinnedWidgetTypes returns the WidgetTypes of every release the
	// project pins: the palette of its Scenes.
	FindPinnedWidgetTypes(ctx context.Context, projectID uuid.UUID) ([]appdto.ReleasedWidgetType, error)
}

type libraryPinServiceImpl struct {
	content   ProjectContent
	libraries library.LibraryRepository
	releases  library.LibraryReleaseRepository
}

var _ LibraryPinService = (*libraryPinServiceImpl)(nil)

// NewLibraryPinService returns the service of the Library Pins. The events of
// its changes are recorded by the LibraryPin and saved with the Draft
// (ADR 0016); the PortBindings a switch breaks are removed in the same change
// (ADR 0012).
func NewLibraryPinService(aContent ProjectContent, someLibraries library.LibraryRepository, someReleases library.LibraryReleaseRepository) LibraryPinService {
	return &libraryPinServiceImpl{content: aContent, libraries: someLibraries, releases: someReleases}
}

func (s libraryPinServiceImpl) FindLibraryPins(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.LibraryPin, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	list, err := scope.LibraryPins().FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find library pins in repository: %w", err)
	}
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	pinned, err := s.pinnedReleases(ctx, list)
	if err != nil {
		return nil, err
	}
	return appdto.NewLibraryPinList(list, version, pinned), nil
}

// pinnedReleases reads the Release each pin names and the title of its Commit,
// in the order of the pins: the provenance a Project sees of what it pinned
// (task 72_5). A Release this server does not have is left nil.
func (s libraryPinServiceImpl) pinnedReleases(ctx context.Context, list []librarypin.LibraryPin) ([]*appdto.PinRelease, error) {
	pinned := make([]*appdto.PinRelease, len(list))
	for i, pin := range list {
		found, err := s.pinnedRelease(ctx, pin)
		if errors.Is(err, library.ErrReleaseNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		title, err := publishedCommitTitle(ctx, s.content.Drafts(), pin.LibraryID(), found)
		if err != nil {
			return nil, err
		}
		pinned[i] = &appdto.PinRelease{Release: found, Title: title}
	}
	return pinned, nil
}

func (s libraryPinServiceImpl) FindPinnedWidgetTypes(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.ReleasedWidgetType, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	list, err := scope.LibraryPins().FindPinnedWidgetTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find pinned widget types in repository: %w", err)
	}
	return appdto.NewReleasedWidgetTypeList(list), nil
}

// openPin opens the live project rawProjectID and finds its library
// rawLibraryID, live and of the Organization.
func (s libraryPinServiceImpl) openPin(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID) (id.ID[project.Project], library.Library, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return projectID, nil, err
	}
	lib, err := findOwnLibrary(ctx, s.libraries, s.content.organization, id.NewID(id.IDWithUUID[library.Library](rawLibraryID)))
	if err != nil {
		return projectID, nil, err
	}
	return projectID, lib, nil
}

// findRelease returns the release rawNumber of lib that the client named; a
// missing one is invalid input.
func (s libraryPinServiceImpl) findRelease(ctx context.Context, lib library.Library, rawNumber int) (library.LibraryRelease, error) {
	number, err := library.NewReleaseNumber(rawNumber)
	if err != nil {
		return nil, fmt.Errorf("release: %w", invalidInput(err))
	}
	release, err := s.releases.FindByNumber(ctx, lib.ID(), number)
	if errors.Is(err, library.ErrReleaseNotFound) {
		return nil, fmt.Errorf("release: %w", invalidInput(err))
	}
	if err != nil {
		return nil, fmt.Errorf("cannot find release: %w", err)
	}
	return release, nil
}

// pinnedRelease returns the release pin pins.
func (s libraryPinServiceImpl) pinnedRelease(ctx context.Context, pin librarypin.LibraryPin) (library.LibraryRelease, error) {
	release, err := s.releases.FindByNumber(ctx, pin.LibraryID(), pin.Release())
	if err != nil {
		return nil, fmt.Errorf("cannot find pinned release: %w", err)
	}
	return release, nil
}

func (s libraryPinServiceImpl) PutLibraryPin(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID, rawRelease, rawVersion int) (appdto.LibraryPin, bool, error) {
	projectID, lib, err := s.openPin(ctx, rawProjectID, rawLibraryID)
	if err != nil {
		return appdto.LibraryPin{}, false, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.LibraryPin{}, false, err
	}
	release, err := s.findRelease(ctx, lib, rawRelease)
	if err != nil {
		return appdto.LibraryPin{}, false, fmt.Errorf("cannot pin library: %w", err)
	}
	var saved librarypin.LibraryPin
	added := false
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Pin library "+lib.Name().String(),
		func(session *draft.Session) error {
			var err error
			saved, added, err = session.PutLibraryPin(ctx, lib.ID(), release.Number())
			return err
		})
	if err != nil {
		if errors.Is(err, library.ErrReleaseNotFound) {
			return appdto.LibraryPin{}, false, fmt.Errorf("release: %w", invalidInput(err))
		}
		return appdto.LibraryPin{}, false, fmt.Errorf("cannot save library pin: %w", err)
	}
	title, err := publishedCommitTitle(ctx, s.content.Drafts(), lib.ID(), release)
	if err != nil {
		return appdto.LibraryPin{}, false, err
	}
	return appdto.NewLibraryPin(saved, result.Version().Number(), &appdto.PinRelease{Release: release, Title: title}), added, nil
}

// DeleteLibraryPin removes the pin of the library, unless Widgets of the
// project are of its WidgetTypes, stating the Version of the Draft.
func (s libraryPinServiceImpl) DeleteLibraryPin(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID, rawVersion int) (appdto.LibraryPin, error) {
	projectID, lib, err := s.openPin(ctx, rawProjectID, rawLibraryID)
	if err != nil {
		return appdto.LibraryPin{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.LibraryPin{}, err
	}
	var deleted librarypin.LibraryPin
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Unpin library "+lib.Name().String(),
		func(session *draft.Session) error {
			var err error
			deleted, err = session.DeleteLibraryPin(ctx, lib.ID())
			return err
		})
	if err != nil {
		return appdto.LibraryPin{}, fmt.Errorf("cannot delete library pin: %w", err)
	}
	var pinned *appdto.PinRelease
	if release, err := s.releases.FindByNumber(ctx, deleted.LibraryID(), deleted.Release()); err == nil {
		title, err := publishedCommitTitle(ctx, s.content.Drafts(), deleted.LibraryID(), release)
		if err != nil {
			return appdto.LibraryPin{}, err
		}
		pinned = &appdto.PinRelease{Release: release, Title: title}
	}
	return appdto.NewLibraryPin(deleted, result.Version().Number(), pinned), nil
}

// PinImpact previews what pinning release instead (nil: removing the pin)
// would do to the project's Widgets as they are now: the same function
// refuses the change itself (librarypin.ImpactOf). Removing a pin the
// project does not have is librarypin.ErrLibraryPinNotFound, as the removal.
func (s libraryPinServiceImpl) PinImpact(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID, rawRelease *int) (appdto.PinImpact, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.PinImpact{}, err
	}
	lib, err := findOwnLibrary(ctx, s.libraries, s.content.organization, id.NewID(id.IDWithUUID[library.Library](rawLibraryID)))
	if err != nil {
		return appdto.PinImpact{}, err
	}
	var from, to library.LibraryRelease
	found, err := scope.LibraryPins().FindByLibrary(ctx, lib.ID())
	switch {
	case err == nil:
		if from, err = s.pinnedRelease(ctx, found); err != nil {
			return appdto.PinImpact{}, err
		}
	case rawRelease == nil || !errors.Is(err, librarypin.ErrLibraryPinNotFound):
		return appdto.PinImpact{}, fmt.Errorf("error on find library pin in repository: %w", err)
	}
	if rawRelease != nil {
		if to, err = s.findRelease(ctx, lib, *rawRelease); err != nil {
			return appdto.PinImpact{}, err
		}
	}

	scenes, err := scope.Scenes().FindAll(ctx)
	if err != nil {
		return appdto.PinImpact{}, fmt.Errorf("error on find scenes in repository: %w", err)
	}
	var widgets []librarypin.PlacedWidget
	var all []scene.Widget
	for _, sc := range scenes {
		for _, w := range sc.Widgets() {
			widgets = append(widgets, librarypin.PlacedWidget{SceneID: sc.ID(), Widget: w})
			all = append(all, w)
		}
	}
	tagTypes, err := loadTagTypes(ctx, scope, scene.BoundTagIDs(all...))
	if err != nil {
		return appdto.PinImpact{}, err
	}
	return appdto.NewPinImpact(librarypin.ImpactOf(widgets, from, to, tagTypes)), nil
}

// loadTagTypes reads, in one query, the TagTypes of the Tags with the given
// IDs of scope's Project, through its tag repository: a Tag that does not
// exist there is left out, for the Scene to judge the binding to it.
func loadTagTypes(ctx context.Context, scope ProjectScope, tagIDs []id.ID[tag.Tag]) (scene.TagTypes, error) {
	types, err := scope.Tags().FindTypesByIDs(ctx, tagIDs)
	if err != nil {
		return scene.TagTypes{}, fmt.Errorf("cannot read tag types: %w", err)
	}
	return scene.NewTagTypes(scope.ProjectID(), types), nil
}
