package application

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// missingLibraryReleases returns the Library Pins of a Deployment's
// ProjectFile that this server cannot write, by Library ID: the Library is not
// live here, or it has no such Release (task 71). A Deployment deployed from a
// file elsewhere may name both; a Library removed after the Deployment was
// made leaves the second.
func missingLibraryReleases(ctx context.Context, libraries library.LibraryRepository,
	releases library.LibraryReleaseRepository, file projectv0.ProjectFile) (map[uuid.UUID]int, error) {
	missing := make(map[uuid.UUID]int, len(file.LibraryPins))
	for _, pin := range file.LibraryPins {
		libraryID := id.NewID(id.IDWithUUID[library.Library](pin.LibraryID))
		if _, err := libraries.FindByID(ctx, libraryID); err != nil {
			if errors.Is(err, library.ErrLibraryNotFound) {
				missing[pin.LibraryID] = pin.Release
				continue
			}
			return nil, fmt.Errorf("cannot read library %s: %w", pin.LibraryID, err)
		}
		number, err := library.NewReleaseNumber(pin.Release)
		if err != nil {
			missing[pin.LibraryID] = pin.Release
			continue
		}
		if _, err := releases.FindByNumber(ctx, libraryID, number); err != nil {
			if errors.Is(err, library.ErrReleaseNotFound) {
				missing[pin.LibraryID] = pin.Release
				continue
			}
			return nil, fmt.Errorf("cannot read release %d of library %s: %w", pin.Release, pin.LibraryID, err)
		}
	}
	return missing, nil
}

// dropMissingLibraryReleases returns the content of a Draft restored from file
// without the pins missing names, and a warning for each (task 71): the
// WidgetTypes the document takes from those Libraries go with the pins, and so
// do the Widgets of them — the Draft cannot hold either without the pin, and a
// Discard must leave it writable. Nothing else of the document changes, so the
// rest of the configuration is restored as it was.
func dropMissingLibraryReleases(content draft.Content, file projectv0.ProjectFile,
	missing map[uuid.UUID]int) (draft.Content, []appdto.DiscardWarning) {
	dropped := make(map[uuid.UUID]int, len(missing))
	if len(missing) > 0 {
		droppedTypes := make(map[uuid.UUID]uuid.UUID, len(file.WidgetTypes))
		for _, wt := range file.WidgetTypes {
			if _, ok := missing[wt.LibraryID]; ok {
				droppedTypes[wt.ID] = wt.LibraryID
			}
		}
		scenes := make([]scene.Scene, len(content.Scenes))
		for i, sc := range content.Scenes {
			for _, w := range sc.Widgets() {
				libraryID, ok := droppedTypes[w.TypeID().UUID()]
				if !ok {
					continue
				}
				dropped[libraryID]++
				// RemoveWidget fails only for an ID the scene does not hold, and
				// every widget the loop reads is its own.
				if without, _, err := sc.RemoveWidget(w.ID()); err == nil {
					sc = without
				}
			}
			scenes[i] = sc
		}
		content.Scenes = scenes
		keptPins := make([]librarypin.LibraryPin, 0, len(content.LibraryPins))
		for _, pin := range content.LibraryPins {
			if _, ok := missing[pin.LibraryID().UUID()]; ok {
				continue
			}
			keptPins = append(keptPins, pin)
		}
		content.LibraryPins = keptPins
	}
	warnings := make([]appdto.DiscardWarning, 0, len(missing))
	for libraryID, release := range missing {
		warnings = append(warnings, appdto.DiscardWarning{
			LibraryID: libraryID, Release: release, DroppedWidgets: dropped[libraryID],
		})
	}
	sort.Slice(warnings, func(i, j int) bool {
		return warnings[i].LibraryID.String() < warnings[j].LibraryID.String()
	})
	return content, warnings
}
