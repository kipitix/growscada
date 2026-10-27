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
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// libraryPinRepositoryPostgresImpl is the librarypin.Repository of one
// Project: every query names project_id. A pin is identified by its Library
// within the Project.
type libraryPinRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
	store   aggregateStore[librarypin.LibraryPin]
}

var _ librarypin.Repository = (*libraryPinRepositoryPostgresImpl)(nil)

// NewLibraryPinRepositoryPostgres returns the repository of the Library Pins
// of the Project aProjectID (ProjectScopes.ForProject is how the server opens
// one).
func NewLibraryPinRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], opts ...Option) librarypin.Repository {
	o := newOptions(opts)
	return &libraryPinRepositoryPostgresImpl{
		db:      aDb,
		project: aProjectID,
		store: aggregateStore[librarypin.LibraryPin]{
			db: aDb, table: tableNameLibraryPins, project: &aProjectID,
			notFound: librarypin.ErrLibraryPinNotFound, conflict: librarypin.ErrLibraryPinConflict,
			onCommit: o.onCommit,
		},
	}
}

func (r libraryPinRepositoryPostgresImpl) checkProject(pin librarypin.LibraryPin) error {
	return r.store.checkProject(pin.ProjectID())
}

// Save inserts a new pin while the Project and the Library are live, each
// row locked for share (ADR 0012); it updates a pin once its row is locked
// for update, its version checked and check passed.
func (r libraryPinRepositoryPostgresImpl) Save(ctx context.Context, pin librarypin.LibraryPin, check librarypin.WidgetsCheck) (librarypin.LibraryPin, error) {
	if err := r.checkProject(pin); err != nil {
		return nil, err
	}
	current := pin.Version()
	var next version.Version[librarypin.LibraryPin]
	switch {
	case current == version.Initial[librarypin.LibraryPin]():
		next = version.Committed[librarypin.LibraryPin]()
	case current.IsCommitted():
		next = current.Next()
	default:
		return nil, fmt.Errorf("undefined behavior with version %d", current.Number())
	}

	err := r.store.inTx(ctx, pin.PendingEvents(), func(tx *sql.Tx) error {
		if current == version.Initial[librarypin.LibraryPin]() {
			if err := r.store.lockLiveProject(ctx, tx); err != nil {
				return err
			}
			// No pin is added to a library once it is marked deleted.
			if _, err := lockLibraryRow(ctx, tx, pin.LibraryID()); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO engineering.library_pins (project_id, library_id, release_number, version) VALUES ($1, $2, $3, $4)`,
				r.project.UUID(), pin.LibraryID().UUID(), pin.Release().Int(), next.Number(),
			)
			return pinWriteError(pin, err)
		}
		if err := r.lockForChange(ctx, tx, pin.LibraryID(), current, check); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE engineering.library_pins SET release_number = $1, version = $2 WHERE project_id = $3 AND library_id = $4`,
			pin.Release().Int(), next.Number(), r.project.UUID(), pin.LibraryID().UUID(),
		)
		return pinWriteError(pin, err)
	})
	if err != nil {
		return nil, err
	}
	return librarypin.ReconstituteLibraryPin(r.project, pin.LibraryID(), pin.Release(), next), nil
}

// Delete removes the pin once its row is locked for update, its version
// checked and check passed: check was built from the release the pin
// pinned when it was read.
func (r libraryPinRepositoryPostgresImpl) Delete(ctx context.Context, pin librarypin.LibraryPin, check librarypin.WidgetsCheck) error {
	if err := r.checkProject(pin); err != nil {
		return err
	}
	current := pin.Version()
	return r.store.inTx(ctx, pin.PendingEvents(), func(tx *sql.Tx) error {
		if err := r.lockForChange(ctx, tx, pin.LibraryID(), current, check); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`DELETE FROM engineering.library_pins WHERE project_id = $1 AND library_id = $2`,
			r.project.UUID(), pin.LibraryID().UUID(),
		)
		if err != nil {
			return fmt.Errorf("cannot delete library pin: %w", err)
		}
		return nil
	})
}

// lockForChange locks the pin's row for update in tx — a write of a Scene
// that adds or retypes a Widget locks it for share (checkPinned) —, checks
// its version, then runs check on the Project's Widgets as they are now.
func (r libraryPinRepositoryPostgresImpl) lockForChange(ctx context.Context, tx *sql.Tx, libraryID id.ID[library.Library], expected version.Version[librarypin.LibraryPin], check librarypin.WidgetsCheck) error {
	var stored int
	err := tx.QueryRowContext(ctx,
		`SELECT version FROM engineering.library_pins WHERE project_id = $1 AND library_id = $2 FOR UPDATE`,
		r.project.UUID(), libraryID.UUID(),
	).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: library %s", librarypin.ErrLibraryPinNotFound, libraryID)
	}
	if err != nil {
		return fmt.Errorf("cannot lock library pin: %w", err)
	}
	if stored != expected.Number() {
		return librarypin.ErrLibraryPinConflict
	}
	if check == nil {
		return nil
	}
	widgets, err := r.readWidgets(ctx, tx)
	if err != nil {
		return err
	}
	return check(widgets)
}

// readWidgets reads every Widget of the Project with its Scene.
func (r libraryPinRepositoryPostgresImpl) readWidgets(ctx context.Context, tx *sql.Tx) ([]librarypin.PlacedWidget, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT `+selectWidgetColumns+` FROM engineering.widgets WHERE project_id = $1 ORDER BY pk_id`, r.project.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widgets of project: %w", err)
	}
	defer rows.Close()
	var widgets []librarypin.PlacedWidget
	for rows.Next() {
		w, sceneID, err := scanWidget(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("error scanning widget: %w", err)
		}
		widgets = append(widgets, librarypin.PlacedWidget{SceneID: id.NewID(id.IDWithUUID[scene.Scene](sceneID)), Widget: w})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over widgets: %w", err)
	}
	return widgets, nil
}

// pinWriteError maps a failed write of a library_pins row.
func pinWriteError(pin librarypin.LibraryPin, err error) error {
	switch {
	case err == nil:
		return nil
	case isUniqueViolation(err, constraintLibraryPinsProjectLibrary):
		return fmt.Errorf("%w: library %s", librarypin.ErrLibraryPinExists, pin.LibraryID())
	case isForeignKeyViolation(err, constraintLibraryPinsRelease):
		return fmt.Errorf("%w: %s of library %s", library.ErrReleaseNotFound, pin.Release(), pin.LibraryID())
	}
	return fmt.Errorf("cannot write library pin: %w", err)
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
		`SELECT library_id, release_number, version FROM engineering.library_pins WHERE project_id = $1 `+and+` ORDER BY pk_id`,
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
			rawVersion   int
		)
		if err := rows.Scan(&rawLibraryID, &rawRelease, &rawVersion); err != nil {
			return nil, fmt.Errorf("error scanning library pin: %w", err)
		}
		release, err := library.NewReleaseNumber(rawRelease)
		if err != nil {
			return nil, fmt.Errorf("cannot create release number: %w", err)
		}
		ver, err := version.New(version.WithNumber[librarypin.LibraryPin](rawVersion))
		if err != nil {
			return nil, fmt.Errorf("cannot create library pin version: %w", err)
		}
		pins = append(pins, librarypin.ReconstituteLibraryPin(r.project, id.NewID(id.IDWithUUID[library.Library](rawLibraryID)), release, ver))
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

// checkPinned locks for share, in tx, the pin of the Project projectID of the
// Library of the WidgetType typeID, and fails with
// library.ErrWidgetTypeNotFound unless the release it pins holds the
// WidgetType: a write of a Scene that adds a Widget or changes its
// WidgetType, against a concurrent switch or removal of the pin, which locks
// the row for update (ADR 0012). The pin is locked alone, then its release
// read as locked: a lock that waited for a switch rereads the row switched,
// whose release the same statement could not join anew.
func checkPinned(ctx context.Context, tx *sql.Tx, projectID id.ID[project.Project], typeID id.ID[library.WidgetType]) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT library_id, release_number FROM engineering.library_pins
		 WHERE project_id = $1
		   AND library_id IN (SELECT library_id FROM engineering.released_widget_types WHERE id = $2)
		 FOR SHARE`,
		projectID.UUID(), typeID.UUID(),
	)
	if err != nil {
		return fmt.Errorf("cannot lock the pin of widget type %s: %w", typeID, err)
	}
	var libraries []string
	var releases []int64
	for rows.Next() {
		var libraryID string
		var release int64
		if err := rows.Scan(&libraryID, &release); err != nil {
			rows.Close()
			return fmt.Errorf("cannot lock the pin of widget type %s: %w", typeID, err)
		}
		libraries = append(libraries, libraryID)
		releases = append(releases, release)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot lock the pin of widget type %s: %w", typeID, err)
	}
	var pinned bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM engineering.released_widget_types
		                WHERE id = $1 AND (library_id, release_number) IN (SELECT unnest($2::uuid[]), unnest($3::int[])))`,
		typeID.UUID(), pq.Array(libraries), pq.Array(releases),
	).Scan(&pinned); err != nil {
		return fmt.Errorf("cannot check the pin of widget type %s: %w", typeID, err)
	}
	if !pinned {
		return id.NotFound(library.ErrWidgetTypeNotFound, typeID)
	}
	return nil
}
