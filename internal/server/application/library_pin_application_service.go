package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// LibraryPinService is the service interface for working with the Library
// Pins of a project's Draft and, through them, the WidgetTypes its Widgets
// may be of (ADR 0007, ADR 0012). Every method takes the project first; a
// project that does not exist (or is deleted) is an error wrapping
// project.ErrProjectNotFound, a library that does not exist one wrapping
// library.ErrLibraryNotFound.
type LibraryPinService interface {
	FindLibraryPins(ctx context.Context, projectID uuid.UUID) ([]appdto.LibraryPin, error)
	// PutLibraryPin pins the release of the library: it adds the pin when
	// version is 0, else switches it, expecting its Version. The bool
	// reports whether the pin was added.
	PutLibraryPin(ctx context.Context, projectID, libraryID uuid.UUID, release, version int) (appdto.LibraryPin, bool, error)
	DeleteLibraryPin(ctx context.Context, projectID, libraryID uuid.UUID) (appdto.LibraryPin, error)
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

// NewLibraryPinService returns the service of the Library Pins. The events
// of its changes are recorded by the LibraryPin and saved with it
// (ADR 0008); the PortBindings a switch breaks are removed by the policy of
// NewWidgetReconciliationPolicy.
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
	return appdto.NewLibraryPinList(list), nil
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
func (s libraryPinServiceImpl) openPin(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID) (ProjectScope, library.Library, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return nil, nil, err
	}
	lib, err := findOwnLibrary(ctx, s.libraries, s.content.organization, id.NewID(id.IDWithUUID[library.Library](rawLibraryID)))
	if err != nil {
		return nil, nil, err
	}
	return scope, lib, nil
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

// noOrphans is the check of a switch from one release to another (to nil
// for a removal): it refuses leaving Widgets without their WidgetType.
func noOrphans(projectID id.ID[project.Project], from, to library.LibraryRelease) librarypin.WidgetsCheck {
	return func(widgets []librarypin.PlacedWidget) error {
		return librarypin.ImpactOf(widgets, from, to, scene.NewTagTypes(projectID, nil)).Check()
	}
}

func (s libraryPinServiceImpl) PutLibraryPin(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID, rawRelease, rawExpectedVersion int) (appdto.LibraryPin, bool, error) {
	scope, lib, err := s.openPin(ctx, rawProjectID, rawLibraryID)
	if err != nil {
		return appdto.LibraryPin{}, false, err
	}
	// A negative version is no version at all: the client's input, not a conflict.
	expected, err := version.New(version.WithNumber[librarypin.LibraryPin](rawExpectedVersion))
	if err != nil {
		return appdto.LibraryPin{}, false, invalidInput(fmt.Errorf("version: %w", err))
	}
	repo := scope.LibraryPins()
	found, err := repo.FindByLibrary(ctx, lib.ID())
	switch {
	case errors.Is(err, librarypin.ErrLibraryPinNotFound) && expected == version.Initial[librarypin.LibraryPin]():
		release, err := s.findRelease(ctx, lib, rawRelease)
		if err != nil {
			return appdto.LibraryPin{}, false, fmt.Errorf("cannot pin library: %w", err)
		}
		saved, err := repo.Save(ctx, librarypin.CreateLibraryPin(scope.ProjectID(), release), nil)
		if err != nil {
			return appdto.LibraryPin{}, false, fmt.Errorf("cannot save library pin: %w", err)
		}
		return appdto.NewLibraryPin(saved), true, nil
	case err != nil:
		return appdto.LibraryPin{}, false, fmt.Errorf("error on find library pin in repository: %w", err)
	}

	// A stale write is a conflict whatever it names.
	if err := found.CheckVersion(expected); err != nil {
		return appdto.LibraryPin{}, false, fmt.Errorf("cannot switch library pin: %w", err)
	}
	release, err := s.findRelease(ctx, lib, rawRelease)
	if err != nil {
		return appdto.LibraryPin{}, false, fmt.Errorf("cannot switch library pin: %w", err)
	}
	from, err := s.pinnedRelease(ctx, found)
	if err != nil {
		return appdto.LibraryPin{}, false, err
	}
	switched, err := found.Switch(expected, release)
	if err != nil {
		return appdto.LibraryPin{}, false, fmt.Errorf("cannot switch library pin: %w", err)
	}
	saved, err := repo.Save(ctx, switched, noOrphans(scope.ProjectID(), from, release))
	if err != nil {
		return appdto.LibraryPin{}, false, fmt.Errorf("cannot save library pin: %w", err)
	}
	return appdto.NewLibraryPin(saved), false, nil
}

// DeleteLibraryPin removes the pin of the library, unless Widgets of the
// project are of its WidgetTypes. It expects no Version: a pin switched
// meanwhile is reread with its release and checked again.
func (s libraryPinServiceImpl) DeleteLibraryPin(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID) (appdto.LibraryPin, error) {
	scope, lib, err := s.openPin(ctx, rawProjectID, rawLibraryID)
	if err != nil {
		return appdto.LibraryPin{}, err
	}
	repo := scope.LibraryPins()
	var found librarypin.LibraryPin
	err = retryOnRace(librarypin.ErrLibraryPinConflict, func() error {
		if found, err = repo.FindByLibrary(ctx, lib.ID()); err != nil {
			return fmt.Errorf("error on find library pin in repository: %w", err)
		}
		from, err := s.pinnedRelease(ctx, found)
		if err != nil {
			return err
		}
		if err := repo.Delete(ctx, found.Delete(), noOrphans(scope.ProjectID(), from, nil)); err != nil {
			return fmt.Errorf("cannot delete library pin: %w", err)
		}
		return nil
	})
	if err != nil {
		return appdto.LibraryPin{}, err
	}
	return appdto.NewLibraryPin(found), nil
}

// PinImpact previews what pinning release instead (nil: removing the pin)
// would do to the project's Widgets as they are now: the same function
// refuses the change itself (librarypin.ImpactOf).
func (s libraryPinServiceImpl) PinImpact(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID, rawRelease *int) (appdto.PinImpact, error) {
	scope, lib, err := s.openPin(ctx, rawProjectID, rawLibraryID)
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
	case !errors.Is(err, librarypin.ErrLibraryPinNotFound):
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
