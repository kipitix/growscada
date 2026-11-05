package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// libraryPinRepositoryPostgresImpl reads the Library Pins of one Project:
// every query names project_id. They are entities of the Project's Draft
// (ADR 0016): their writes go through draftRepositoryPostgres.
type libraryPinRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
}

var _ librarypin.Repository = (*libraryPinRepositoryPostgresImpl)(nil)

// NewLibraryPinRepositoryPostgres returns the repository of the pins of the
// Project aProjectID.
func NewLibraryPinRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], _ ...Option) librarypin.Repository {
	return &libraryPinRepositoryPostgresImpl{db: aDb, project: aProjectID}
}

func (r libraryPinRepositoryPostgresImpl) FindAll(ctx context.Context) ([]librarypin.LibraryPin, error) {
	return r.readPins(ctx, ``)
}

func (r libraryPinRepositoryPostgresImpl) FindByLibrary(ctx context.Context, libraryID id.ID[library.Library]) (librarypin.LibraryPin, error) {
	found, err := r.readPins(ctx, `AND library_id = $2`, libraryID.UUID())
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("%w: library %s", librarypin.ErrLibraryPinNotFound, libraryID)
	}
	return found[0], nil
}

func (r libraryPinRepositoryPostgresImpl) readPins(ctx context.Context, and string, args ...any) ([]librarypin.LibraryPin, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT library_id, release_number FROM engineering.library_pins WHERE project_id = $1 `+and+` ORDER BY pk_id`,
		append([]any{r.project.UUID()}, args...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying library pins: %w", err)
	}
	defer rows.Close()
	pins := []librarypin.LibraryPin{}
	for rows.Next() {
		var (
			rawLibraryID uuid.UUID
			rawRelease   int
		)
		if err := rows.Scan(&rawLibraryID, &rawRelease); err != nil {
			return nil, fmt.Errorf("error scanning library pin: %w", err)
		}
		release, err := library.NewReleaseNumber(rawRelease)
		if err != nil {
			return nil, fmt.Errorf("cannot create release number: %w", err)
		}
		pins = append(pins, librarypin.ReconstituteLibraryPin(r.project, id.NewID(id.IDWithUUID[library.Library](rawLibraryID)), release))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over library pins: %w", err)
	}
	return pins, nil
}

// pinnedWidgetTypesQuery selects the released WidgetTypes of the releases
// the Project $1 pins.
const pinnedWidgetTypesQuery = `SELECT ` + selectReleasedWidgetTypeColumnsOf + `
	FROM engineering.library_pins lp
	JOIN engineering.released_widget_types r ON r.library_id = lp.library_id AND r.release_number = lp.release_number
	WHERE lp.project_id = $1`

// selectReleasedWidgetTypeColumnsOf is selectReleasedWidgetTypeColumns of
// the table aliased r.
const selectReleasedWidgetTypeColumnsOf = `r.id, r.library_id, r.name, r.html_template, r.script, r.script_language, r.default_width, r.default_height, r.input_ports`

func (r libraryPinRepositoryPostgresImpl) FindPinnedWidgetTypes(ctx context.Context) ([]library.ReleasedWidgetType, error) {
	return r.readPinned(ctx, ``)
}

func (r libraryPinRepositoryPostgresImpl) FindPinnedWidgetTypesByID(ctx context.Context, typeIDs []id.ID[library.WidgetType]) ([]library.ReleasedWidgetType, error) {
	raw := make([]string, len(typeIDs))
	for i, typeID := range typeIDs {
		raw[i] = typeID.UUID().String()
	}
	return r.readPinned(ctx, ` AND r.id = ANY($2::uuid[])`, pq.Array(raw))
}

// readPinned reads the pinned WidgetTypes matching and, a further condition
// whose arguments are numbered from $2, in the order of their pins.
func (r libraryPinRepositoryPostgresImpl) readPinned(ctx context.Context, and string, args ...any) ([]library.ReleasedWidgetType, error) {
	rows, err := r.db.QueryContext(ctx, pinnedWidgetTypesQuery+and+` ORDER BY lp.pk_id, r.pk_id`, append([]any{r.project.UUID()}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("error querying pinned widget types: %w", err)
	}
	defer rows.Close()
	types := []library.ReleasedWidgetType{}
	for rows.Next() {
		wt, err := scanReleasedWidgetType(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("error scanning released widget type: %w", err)
		}
		types = append(types, wt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over pinned widget types: %w", err)
	}
	return types, nil
}

func (r libraryPinRepositoryPostgresImpl) FindPinnedWidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) (library.ReleasedWidgetType, error) {
	wt, err := scanReleasedWidgetType(r.db.QueryRowContext(ctx, pinnedWidgetTypesQuery+` AND r.id = $2`, r.project.UUID(), typeID.UUID()).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return library.ReleasedWidgetType{}, id.NotFound(library.ErrWidgetTypeNotFound, typeID)
	}
	if err != nil {
		return library.ReleasedWidgetType{}, fmt.Errorf("error scanning released widget type: %w", err)
	}
	return wt, nil
}
