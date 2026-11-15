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
	"github.com/kipitix/growscada/internal/server/domain/libraryimport"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// missingLibraryImports returns the Library Imports of a Deployment's
// ProjectFile that this server cannot write, in the order of their Library
// IDs: the Library is not live here, or its Draft has no such saved state
// (task 71, task 85_6). A Deployment deployed from a file elsewhere may name
// both; a Library removed after the Deployment was made leaves the second.
func missingLibraryImports(ctx context.Context, libraries library.LibraryRepository,
	drafts draft.Repository, file projectv0.ProjectFile) ([]appdto.MissingImport, error) {
	byLibrary := make(map[uuid.UUID]int, len(file.LibraryImports))
	for _, imported := range file.LibraryImports {
		libraryID := id.NewID(id.IDWithUUID[library.Library](imported.LibraryID))
		if _, err := libraries.FindByID(ctx, libraryID); err != nil {
			if errors.Is(err, library.ErrLibraryNotFound) {
				byLibrary[imported.LibraryID] = imported.SavedState
				continue
			}
			return nil, fmt.Errorf("cannot read library %s: %w", imported.LibraryID, err)
		}
		if _, err := drafts.FindSavedState(ctx, draft.LibraryScope(libraryID), imported.SavedState); err != nil {
			if errors.Is(err, draft.ErrSavedStateNotFound) {
				byLibrary[imported.LibraryID] = imported.SavedState
				continue
			}
			return nil, fmt.Errorf("cannot read saved state %d of library %s: %w",
				imported.SavedState, imported.LibraryID, err)
		}
	}
	missing := make([]appdto.MissingImport, 0, len(byLibrary))
	for libraryID, savedState := range byLibrary {
		missing = append(missing, appdto.MissingImport{LibraryID: libraryID, SavedState: savedState})
	}
	sort.Slice(missing, func(i, j int) bool {
		return missing[i].LibraryID.String() < missing[j].LibraryID.String()
	})
	return missing, nil
}

// dropMissingLibraryImports returns the content of a Draft restored from file
// without the imports missing names, and a warning for each (task 71): the
// WidgetTypes the document takes from those Libraries go with the imports, and
// so do the Widgets of them — the Draft cannot hold either without the import,
// and a Discard must leave it writable. Nothing else of the document changes,
// so the rest of the configuration is restored as it was.
func dropMissingLibraryImports(content draft.Content, file projectv0.ProjectFile,
	missing []appdto.MissingImport) (draft.Content, []appdto.DiscardWarning) {
	dropped := make(map[uuid.UUID]int, len(missing))
	if len(missing) > 0 {
		gone := make(map[uuid.UUID]struct{}, len(missing))
		for _, imported := range missing {
			gone[imported.LibraryID] = struct{}{}
		}
		droppedTypes := make(map[uuid.UUID]uuid.UUID, len(file.WidgetTypes))
		for _, wt := range file.WidgetTypes {
			if _, ok := gone[wt.LibraryID]; ok {
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
		keptImports := make([]libraryimport.LibraryImport, 0, len(content.LibraryImports))
		for _, imported := range content.LibraryImports {
			if _, ok := gone[imported.LibraryID().UUID()]; ok {
				continue
			}
			keptImports = append(keptImports, imported)
		}
		content.LibraryImports = keptImports
	}
	warnings := make([]appdto.DiscardWarning, 0, len(missing))
	for _, imported := range missing {
		warnings = append(warnings, appdto.DiscardWarning{
			LibraryID: imported.LibraryID, SavedState: imported.SavedState, DroppedWidgets: dropped[imported.LibraryID],
		})
	}
	return content, warnings
}
