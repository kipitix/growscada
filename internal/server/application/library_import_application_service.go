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
	"github.com/kipitix/growscada/internal/server/domain/libraryimport"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// MissingCommitError names a saved state a request names — in the body of a
// Library Import — that the Library's Draft does not have. It matches
// draft.ErrCommitNotFound, and it carries the number, which the path does not:
// the API's 404 names it (task 85_6).
type MissingCommitError struct {
	Number int
}

func (e MissingCommitError) Error() string {
	return fmt.Sprintf("%v: saved state %d", draft.ErrCommitNotFound, e.Number)
}

func (e MissingCommitError) Is(target error) bool { return target == draft.ErrCommitNotFound }

// LibraryImportService is the service interface for working with the Library
// Imports of a project's Draft and, through them, the WidgetTypes its Widgets
// may be of (ADR 0007, ADR 0012; task 85_6). Every write states the Version of
// the Draft (ADR 0016). Every method takes the project first; a project that
// does not exist (or is deleted) is an error wrapping project.ErrProjectNotFound,
// a library that does not exist one wrapping library.ErrLibraryNotFound.
type LibraryImportService interface {
	FindLibraryImports(ctx context.Context, projectID uuid.UUID) ([]appdto.LibraryImport, error)
	// PutLibraryImport imports the saved state commit of the library: it adds
	// the import when the project imports none of the library, else switches
	// it, stating the Version of the Draft. The bool reports whether the
	// import was added.
	PutLibraryImport(ctx context.Context, projectID, libraryID uuid.UUID, commit, version int) (appdto.LibraryImport, bool, error)
	DeleteLibraryImport(ctx context.Context, projectID, libraryID uuid.UUID, version int) (appdto.LibraryImport, error)
	// ImportImpact previews switching the import of the library to commit, or
	// removing it for nil.
	ImportImpact(ctx context.Context, projectID, libraryID uuid.UUID, commit *int) (appdto.ImportImpact, error)
	// FindImportedWidgetTypes returns the WidgetTypes of every saved state the
	// project imports: the palette of its Scenes.
	FindImportedWidgetTypes(ctx context.Context, projectID uuid.UUID) ([]appdto.WidgetTypeCopy, error)
}

type libraryImportServiceImpl struct {
	content   ProjectContent
	libraries library.LibraryRepository
	labels    LabelService
}

var _ LibraryImportService = (*libraryImportServiceImpl)(nil)

// NewLibraryImportService returns the service of the Library Imports. The
// events of its changes are recorded by the LibraryImport and saved with the
// Draft (ADR 0016); the PortBindings a switch breaks are removed in the same
// change (ADR 0012). The saved states an import names are the Commits of the
// Libraries' Drafts, read through the ProjectContent's Draft repository.
func NewLibraryImportService(aContent ProjectContent, someLibraries library.LibraryRepository, someLabels LabelService) LibraryImportService {
	return &libraryImportServiceImpl{content: aContent, libraries: someLibraries, labels: someLabels}
}

func (s libraryImportServiceImpl) FindLibraryImports(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.LibraryImport, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	list, err := scope.LibraryImports().FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find library imports in repository: %w", err)
	}
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	result := make([]appdto.LibraryImport, len(list))
	for i, imported := range list {
		commit, labels, err := s.provenance(ctx, imported)
		if err != nil {
			return nil, err
		}
		result[i] = appdto.NewLibraryImport(imported, version, commit, labels)
	}
	return result, nil
}

// provenance reads the Commit an import names and the labels it carries: the
// saved state a Project sees of what it imported (task 85_6). A Commit this
// server cannot read — its Library is being removed — leaves both empty.
func (s libraryImportServiceImpl) provenance(ctx context.Context, imported libraryimport.LibraryImport) (draft.Commit, []appdto.Label, error) {
	commit, err := s.libraryCommit(ctx, imported.LibraryID(), imported.CommitNumber())
	if err != nil {
		if !errors.Is(err, draft.ErrCommitNotFound) && !errors.Is(err, library.ErrLibraryNotFound) {
			return nil, nil, err
		}
		return nil, nil, nil
	}
	labels, err := s.labelsOf(ctx, imported.LibraryID(), imported.CommitNumber())
	if err != nil {
		return nil, nil, err
	}
	return commit, labels, nil
}

// libraryCommit returns the saved state number of the Library, or nil for one
// this server does not have.
func (s libraryImportServiceImpl) libraryCommit(ctx context.Context, libraryID id.ID[library.Library], number int) (draft.Commit, error) {
	commit, err := s.content.Drafts().FindCommit(ctx, draft.LibraryScope(libraryID), number)
	if err != nil {
		if errors.Is(err, draft.ErrCommitNotFound) {
			return nil, MissingCommitError{Number: number}
		}
		if errors.Is(err, library.ErrLibraryNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("cannot read saved state %d of library %s: %w", number, libraryID, err)
	}
	return commit, nil
}

func (s libraryImportServiceImpl) FindImportedWidgetTypes(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.WidgetTypeCopy, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	list, err := scope.LibraryImports().FindImportedWidgetTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find imported widget types in repository: %w", err)
	}
	return appdto.NewWidgetTypeCopyList(list), nil
}

// openImport opens the live project rawProjectID and finds its library
// rawLibraryID, live and of the Organization.
func (s libraryImportServiceImpl) openImport(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID) (id.ID[project.Project], library.Library, error) {
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

func (s libraryImportServiceImpl) PutLibraryImport(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID, rawCommit, rawVersion int) (appdto.LibraryImport, bool, error) {
	projectID, lib, err := s.openImport(ctx, rawProjectID, rawLibraryID)
	if err != nil {
		return appdto.LibraryImport{}, false, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.LibraryImport{}, false, err
	}
	commit, err := s.libraryCommit(ctx, lib.ID(), rawCommit)
	if err != nil {
		return appdto.LibraryImport{}, false, fmt.Errorf("cannot import library: %w", err)
	}
	var saved libraryimport.LibraryImport
	added := false
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Import library "+lib.Name().String(),
		func(session *draft.Session) error {
			var err error
			saved, added, err = session.PutLibraryImport(ctx, lib.ID(), rawCommit)
			return err
		})
	if err != nil {
		// The saved state was read before the write and can be gone by it: the
		// 404 must name the number the body did, which the path does not carry.
		if errors.Is(err, draft.ErrCommitNotFound) {
			return appdto.LibraryImport{}, false, MissingCommitError{Number: rawCommit}
		}
		return appdto.LibraryImport{}, false, fmt.Errorf("cannot save library import: %w", err)
	}
	labels, err := s.labelsOf(ctx, lib.ID(), saved.CommitNumber())
	if err != nil {
		return appdto.LibraryImport{}, false, err
	}
	return appdto.NewLibraryImport(saved, result.Version().Number(), commit, labels), added, nil
}

// DeleteLibraryImport removes the import of the library, unless Widgets of the
// project are of its WidgetTypes, stating the Version of the Draft.
func (s libraryImportServiceImpl) DeleteLibraryImport(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID, rawVersion int) (appdto.LibraryImport, error) {
	projectID, lib, err := s.openImport(ctx, rawProjectID, rawLibraryID)
	if err != nil {
		return appdto.LibraryImport{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.LibraryImport{}, err
	}
	var deleted libraryimport.LibraryImport
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Remove library import "+lib.Name().String(),
		func(session *draft.Session) error {
			var err error
			deleted, err = session.DeleteLibraryImport(ctx, lib.ID())
			return err
		})
	if err != nil {
		return appdto.LibraryImport{}, fmt.Errorf("cannot delete library import: %w", err)
	}
	commit, err := s.libraryCommit(ctx, deleted.LibraryID(), deleted.CommitNumber())
	if err != nil && !errors.Is(err, draft.ErrCommitNotFound) && !errors.Is(err, library.ErrLibraryNotFound) {
		return appdto.LibraryImport{}, err
	}
	labels, err := s.labelsOf(ctx, deleted.LibraryID(), deleted.CommitNumber())
	if err != nil {
		return appdto.LibraryImport{}, err
	}
	return appdto.NewLibraryImport(deleted, result.Version().Number(), commit, labels), nil
}

// labelsOf reads the labels of one saved state of the library; a Library this
// server cannot read leaves them empty.
func (s libraryImportServiceImpl) labelsOf(ctx context.Context, libraryID id.ID[library.Library], number int) ([]appdto.Label, error) {
	byNumber, err := s.labels.LabelsOf(ctx, draft.LibraryScope(libraryID))
	if err != nil {
		if errors.Is(err, library.ErrLibraryNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read the labels of library %s: %w", libraryID, err)
	}
	return byNumber[number], nil
}

// ImportImpact previews what importing commit instead (nil: removing the
// import) would do to the project's Widgets as they are now: the same function
// refuses the change itself (libraryimport.ImportImpactOf). Removing an import
// the project does not have is libraryimport.ErrLibraryImportNotFound, as the
// removal.
func (s libraryImportServiceImpl) ImportImpact(ctx context.Context, rawProjectID, rawLibraryID uuid.UUID, rawCommit *int) (appdto.ImportImpact, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.ImportImpact{}, err
	}
	lib, err := findOwnLibrary(ctx, s.libraries, s.content.organization, id.NewID(id.IDWithUUID[library.Library](rawLibraryID)))
	if err != nil {
		return appdto.ImportImpact{}, err
	}
	var from, to *libraryimport.Commit
	found, err := scope.LibraryImports().FindByLibrary(ctx, lib.ID())
	switch {
	case err == nil:
		if from, err = s.importCommitOf(ctx, lib.ID(), found.CommitNumber()); err != nil {
			return appdto.ImportImpact{}, err
		}
	case rawCommit == nil || !errors.Is(err, libraryimport.ErrLibraryImportNotFound):
		return appdto.ImportImpact{}, fmt.Errorf("error on find library import in repository: %w", err)
	}
	if rawCommit != nil {
		if to, err = s.importCommitOf(ctx, lib.ID(), *rawCommit); err != nil {
			return appdto.ImportImpact{}, err
		}
	}

	scenes, err := scope.Scenes().FindAll(ctx)
	if err != nil {
		return appdto.ImportImpact{}, fmt.Errorf("error on find scenes in repository: %w", err)
	}
	var widgets []libraryimport.PlacedWidget
	var all []scene.Widget
	for _, sc := range scenes {
		for _, w := range sc.Widgets() {
			widgets = append(widgets, libraryimport.PlacedWidget{SceneID: sc.ID(), Widget: w})
			all = append(all, w)
		}
	}
	tagTypes, err := loadTagTypes(ctx, scope, scene.BoundTagIDs(all...))
	if err != nil {
		return appdto.ImportImpact{}, err
	}
	return appdto.NewImportImpact(libraryimport.ImportImpactOf(widgets, from, to, tagTypes)), nil
}

// importCommitOf returns the saved state number of the Library as the value an
// import names: its identity and the copies of its WidgetTypes.
func (s libraryImportServiceImpl) importCommitOf(ctx context.Context, libraryID id.ID[library.Library], number int) (*libraryimport.Commit, error) {
	commit, err := s.content.Drafts().FindCommit(ctx, draft.LibraryScope(libraryID), number)
	if err != nil {
		if errors.Is(err, draft.ErrCommitNotFound) {
			return nil, MissingCommitError{Number: number}
		}
		return nil, fmt.Errorf("cannot read saved state %d of library %s: %w", number, libraryID, err)
	}
	types, ok := commit.Content().WidgetTypes()
	if !ok {
		return nil, fmt.Errorf("saved state %d of library %s holds no widget types", number, libraryID)
	}
	value := libraryimport.ReconstituteCommit(commit.ID().UUID(), libraryID, number, types)
	return &value, nil
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
