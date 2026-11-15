package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/libraryimport"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// libraryImportRepositoryPostgresImpl reads the Library Imports of one Project:
// every query names project_id. They are entities of the Project's Draft (ADR
// 0016): their writes go through draftRepositoryPostgres. The WidgetTypes they
// give the Project are the copies the imported saved states hold, decoded from
// the saved state's content (task 85_6).
type libraryImportRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
}

var _ libraryimport.Repository = (*libraryImportRepositoryPostgresImpl)(nil)

// NewLibraryImportRepositoryPostgres returns the repository of the imports of
// the Project aProjectID.
func NewLibraryImportRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], _ ...Option) libraryimport.Repository {
	return &libraryImportRepositoryPostgresImpl{db: aDb, project: aProjectID}
}

func (r libraryImportRepositoryPostgresImpl) FindAll(ctx context.Context) ([]libraryimport.LibraryImport, error) {
	return r.readImports(ctx, ``)
}

func (r libraryImportRepositoryPostgresImpl) FindByLibrary(ctx context.Context, libraryID id.ID[library.Library]) (libraryimport.LibraryImport, error) {
	found, err := r.readImports(ctx, `AND library_id = $2`, libraryID.UUID())
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("%w: library %s", libraryimport.ErrLibraryImportNotFound, libraryID)
	}
	return found[0], nil
}

// readImports reads the imports matching and, a further condition whose
// arguments are numbered from $2, in the order they were added.
func (r libraryImportRepositoryPostgresImpl) readImports(ctx context.Context, and string, args ...any) ([]libraryimport.LibraryImport, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT library_id, commit_id, commit_number FROM engineering.library_imports WHERE project_id = $1 `+and+` ORDER BY pk_id`,
		append([]any{r.project.UUID()}, args...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying library imports: %w", err)
	}
	defer func() { _ = rows.Close() }()
	imports := []libraryimport.LibraryImport{}
	for rows.Next() {
		var (
			rawLibraryID uuid.UUID
			rawCommitID  uuid.UUID
			rawCommit    int
		)
		if err := rows.Scan(&rawLibraryID, &rawCommitID, &rawCommit); err != nil {
			return nil, fmt.Errorf("error scanning library import: %w", err)
		}
		imports = append(imports, libraryimport.ReconstituteLibraryImport(r.project,
			id.NewID(id.IDWithUUID[library.Library](rawLibraryID)), rawCommitID, rawCommit))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over library imports: %w", err)
	}
	return imports, nil
}

// importedWidgetTypesQuery selects the content of the saved states the Project
// $1 imports, in the order of its imports.
const importedWidgetTypesQuery = `SELECT c.content
	FROM engineering.library_imports li
	JOIN engineering.commits c ON c.id = li.commit_id
	WHERE li.project_id = $1
	ORDER BY li.pk_id`

// FindImportedWidgetTypes returns the WidgetTypes of every saved state the
// Project imports, in the order of the imports and of each saved state's
// copies.
func (r libraryImportRepositoryPostgresImpl) FindImportedWidgetTypes(ctx context.Context) ([]library.WidgetTypeCopy, error) {
	return r.readImported(ctx, nil)
}

// FindImportedWidgetTypesByID returns the imported WidgetTypes with the given
// IDs; an ID none of the saved states holds is left out.
func (r libraryImportRepositoryPostgresImpl) FindImportedWidgetTypesByID(ctx context.Context, typeIDs []id.ID[library.WidgetType]) ([]library.WidgetTypeCopy, error) {
	wanted := make(map[uuid.UUID]struct{}, len(typeIDs))
	for _, typeID := range typeIDs {
		wanted[typeID.UUID()] = struct{}{}
	}
	return r.readImported(ctx, wanted)
}

// readImported reads the WidgetTypes of the imported saved states, all of them
// or only those whose IDs are in wanted.
func (r libraryImportRepositoryPostgresImpl) readImported(ctx context.Context, wanted map[uuid.UUID]struct{}) ([]library.WidgetTypeCopy, error) {
	rows, err := r.db.QueryContext(ctx, importedWidgetTypesQuery, r.project.UUID())
	if err != nil {
		return nil, fmt.Errorf("error querying imported widget types: %w", err)
	}
	defer func() { _ = rows.Close() }()
	types := []library.WidgetTypeCopy{}
	for rows.Next() {
		var contentJSON []byte
		if err := rows.Scan(&contentJSON); err != nil {
			return nil, fmt.Errorf("error scanning the content of an imported saved state: %w", err)
		}
		var content draft.CommitContent
		if err := json.Unmarshal(contentJSON, &content); err != nil {
			return nil, fmt.Errorf("cannot decode the content of an imported saved state: %w", err)
		}
		held, ok := content.WidgetTypes()
		if !ok {
			return nil, fmt.Errorf("an imported saved state of project %s holds no widget types", r.project)
		}
		for _, wt := range held {
			if wanted != nil {
				if _, ok := wanted[wt.ID().UUID()]; !ok {
					continue
				}
			}
			types = append(types, wt)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over imported widget types: %w", err)
	}
	return types, nil
}

// FindImportedWidgetType returns the WidgetType typeID held by a saved state
// the Project imports.
func (r libraryImportRepositoryPostgresImpl) FindImportedWidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) (library.WidgetTypeCopy, error) {
	types, err := r.FindImportedWidgetTypesByID(ctx, []id.ID[library.WidgetType]{typeID})
	if err != nil {
		return library.WidgetTypeCopy{}, err
	}
	if len(types) == 0 {
		return library.WidgetTypeCopy{}, id.NotFound(library.ErrWidgetTypeNotFound, typeID)
	}
	return types[0], nil
}

// commitContentOfImport reads the content of a saved state an import names,
// inside the Draft's transaction: the loader decodes the copies an import
// switch and a removal are checked against.
func commitContentOfImport(ctx context.Context, q rowQuerier, libraryID id.ID[library.Library], number int) (draft.CommitContent, uuid.UUID, error) {
	var (
		rawID       uuid.UUID
		contentJSON []byte
	)
	err := q.QueryRowContext(ctx,
		`SELECT c.id, c.content
		 FROM engineering.commits c JOIN engineering.drafts d ON d.id = c.draft_id
		 WHERE d.library_id = $1 AND c.number = $2`, libraryID.UUID(), number,
	).Scan(&rawID, &contentJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return draft.CommitContent{}, uuid.Nil, fmt.Errorf("%w: %d of library %s", draft.ErrCommitNotFound, number, libraryID)
	}
	if err != nil {
		return draft.CommitContent{}, uuid.Nil, fmt.Errorf("cannot read saved state %d of library %s: %w", number, libraryID, err)
	}
	var content draft.CommitContent
	if err := json.Unmarshal(contentJSON, &content); err != nil {
		return draft.CommitContent{}, uuid.Nil, fmt.Errorf("cannot decode saved state %d of library %s: %w", number, libraryID, err)
	}
	return content, rawID, nil
}
