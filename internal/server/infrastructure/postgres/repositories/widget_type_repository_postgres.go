package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// widgetTypeRepositoryPostgresImpl is the WidgetTypeRepository of one
// Library's Draft: every query names library_id, and every write first locks
// the Library's row for share and checks it (lockLibrary).
type widgetTypeRepositoryPostgresImpl struct {
	db      *sql.DB
	library id.ID[library.Library]
	// discard makes the writes a Discard's: made only while one is under
	// way, refused otherwise.
	discard bool
	store   aggregateStore[library.WidgetType]
}

var _ library.WidgetTypeRepository = (*widgetTypeRepositoryPostgresImpl)(nil)

// widgetTypeRepositories opens the WidgetTypeRepository of a Library's Draft.
type widgetTypeRepositories struct {
	db   *sql.DB
	opts []Option
}

var _ library.WidgetTypeRepositories = (*widgetTypeRepositories)(nil)

// NewWidgetTypeRepositories returns the WidgetTypeRepositories over the
// database; the options apply to every repository it opens.
func NewWidgetTypeRepositories(aDb *sql.DB, opts ...Option) library.WidgetTypeRepositories {
	return &widgetTypeRepositories{db: aDb, opts: opts}
}

func (r *widgetTypeRepositories) ForLibrary(libraryID id.ID[library.Library]) library.WidgetTypeRepository {
	return NewWidgetTypeRepositoryPostgres(r.db, libraryID, r.opts...)
}

func (r *widgetTypeRepositories) ForDiscard(libraryID id.ID[library.Library]) library.WidgetTypeRepository {
	return newWidgetTypeRepository(r.db, libraryID, true, r.opts...)
}

// NewWidgetTypeRepositoryPostgres returns the repository of the widget types
// of the Library aLibraryID, whose writes are refused while a Discard is
// under way.
func NewWidgetTypeRepositoryPostgres(aDb *sql.DB, aLibraryID id.ID[library.Library], opts ...Option) library.WidgetTypeRepository {
	return newWidgetTypeRepository(aDb, aLibraryID, false, opts...)
}

func newWidgetTypeRepository(aDb *sql.DB, aLibraryID id.ID[library.Library], discard bool, opts ...Option) *widgetTypeRepositoryPostgresImpl {
	o := newOptions(opts)
	r := &widgetTypeRepositoryPostgresImpl{db: aDb, library: aLibraryID, discard: discard}
	r.store = aggregateStore[library.WidgetType]{
		db: aDb, table: tableNameWidgetTypes, library: &r.library, lock: r.lockLibrary,
		notFound: library.ErrWidgetTypeNotFound, conflict: library.ErrWidgetTypeConflict,
		onCommit: o.onCommit,
	}
	return r
}

// errNotDiscarding refuses a write of a Discard's repository once no Discard
// of its Library is under way.
var errNotDiscarding = errors.New("library is not being discarded")

// lockLibrary locks the row of the repository's Library for share in tx, the
// transaction of a write of one of its widget types, and checks it: live,
// and being discarded exactly when the repository is a Discard's (ADR 0012).
// Marking a Discard updates the row, so it waits for the write, or the write
// for it.
func (r *widgetTypeRepositoryPostgresImpl) lockLibrary(ctx context.Context, tx *sql.Tx) error {
	var discardingTo sql.NullInt64
	err := tx.QueryRowContext(ctx,
		`SELECT discarding_to FROM engineering.libraries WHERE id = $1 AND deleted_at IS NULL FOR SHARE`, r.library.UUID(),
	).Scan(&discardingTo)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return id.NotFound(library.ErrLibraryNotFound, r.library)
	case err != nil:
		return fmt.Errorf("cannot lock library %s: %w", r.library, err)
	case discardingTo.Valid && !r.discard:
		return fmt.Errorf("%w: %s", library.ErrLibraryDiscarding, r.library)
	case !discardingTo.Valid && r.discard:
		return fmt.Errorf("%w: %s", errNotDiscarding, r.library)
	}
	return nil
}

// checkLibrary returns an error unless wt may be written through the
// repository: a widget type of its Library.
func (r *widgetTypeRepositoryPostgresImpl) checkLibrary(wt library.WidgetType) error {
	if wt.LibraryID() != r.library {
		return fmt.Errorf("cannot write widget type %s of library %s through the repository of library %s", wt.ID(), wt.LibraryID(), r.library)
	}
	return nil
}

func (r *widgetTypeRepositoryPostgresImpl) NextID() id.ID[library.WidgetType] {
	return id.NewID[library.WidgetType]()
}

func (r *widgetTypeRepositoryPostgresImpl) Save(ctx context.Context, wt library.WidgetType) (library.WidgetType, error) {
	if err := r.checkLibrary(wt); err != nil {
		return nil, err
	}
	portsJSON, err := marshalInputPorts(wt.InputPorts())
	if err != nil {
		return nil, fmt.Errorf("cannot serialize input ports: %w", err)
	}

	// The row is read back so that the result is what a later read returns.
	var saved library.WidgetType
	_, err = r.store.save(ctx, wt.ID(), wt.Version(), wt.PendingEvents(), rowWrite[library.WidgetType]{
		insert: func(tx *sql.Tx, next version.Version[library.WidgetType]) error {
			row := tx.QueryRowContext(ctx,
				`INSERT INTO engineering.widget_types (id, library_id, name, html_template, script, script_language, default_width, default_height, input_ports, version)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				RETURNING `+selectWidgetTypeColumns,
				wt.ID().UUID(), r.library.UUID(), wt.Name().String(), wt.HtmlTemplate().String(),
				wt.Script().String(), wt.ScriptLanguage().String(),
				wt.DefaultSize().Width(), wt.DefaultSize().Height(),
				portsJSON, next.Number(),
			)
			if saved, err = r.scanWidgetType(row.Scan); err != nil {
				return widgetTypeWriteError("insert new widget type", err)
			}
			return nil
		},
		update: func(tx *sql.Tx, current, next version.Version[library.WidgetType]) (bool, error) {
			row := tx.QueryRowContext(ctx,
				`UPDATE engineering.widget_types
				 SET name = $1, html_template = $2, script = $3, script_language = $4,
				     default_width = $5, default_height = $6, input_ports = $7, version = $8
				 WHERE id = $9 AND version = $10 AND library_id = $11
				 RETURNING `+selectWidgetTypeColumns,
				wt.Name().String(), wt.HtmlTemplate().String(), wt.Script().String(),
				wt.ScriptLanguage().String(),
				wt.DefaultSize().Width(), wt.DefaultSize().Height(),
				portsJSON, next.Number(),
				wt.ID().UUID(), current.Number(), r.library.UUID(),
			)
			saved, err = r.scanWidgetType(row.Scan)
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			if err != nil {
				return false, widgetTypeWriteError("update widget type", err)
			}
			return true, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

// widgetTypeWriteError maps a failed write of a widget_types row: a name
// taken in the library is ErrWidgetTypeNameTaken.
func widgetTypeWriteError(what string, err error) error {
	if isUniqueViolation(err, constraintWidgetTypesName) {
		return library.ErrWidgetTypeNameTaken
	}
	return fmt.Errorf("cannot %s: %w", what, err)
}

// Delete removes the widget type. No Widget refers to it: Widgets are of the
// copies the releases hold.
func (r *widgetTypeRepositoryPostgresImpl) Delete(ctx context.Context, wt library.WidgetType) error {
	if err := r.checkLibrary(wt); err != nil {
		return err
	}
	return r.store.delete(ctx, wt.ID(), wt.Version(), wt.PendingEvents())
}

const selectWidgetTypeColumns = `id, library_id, name, html_template, script, script_language, default_width, default_height, input_ports, version`

func (r *widgetTypeRepositoryPostgresImpl) FindByID(ctx context.Context, anID id.ID[library.WidgetType]) (library.WidgetType, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+selectWidgetTypeColumns+" FROM engineering.widget_types WHERE id = $1 AND library_id = $2",
		anID.UUID(), r.library.UUID(),
	)
	wt, err := r.scanWidgetType(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, id.NotFound(library.ErrWidgetTypeNotFound, anID)
		}
		return nil, fmt.Errorf("error scanning widget type: %w", err)
	}
	return wt, nil
}

func (r *widgetTypeRepositoryPostgresImpl) FindAll(ctx context.Context) ([]library.WidgetType, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+selectWidgetTypeColumns+" FROM engineering.widget_types WHERE library_id = $1 ORDER BY pk_id",
		r.library.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widget types: %w", err)
	}
	defer rows.Close()

	var result []library.WidgetType
	for rows.Next() {
		wt, err := r.scanWidgetType(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("error scanning widget type: %w", err)
		}
		result = append(result, wt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over widget types: %w", err)
	}
	return result, nil
}

// rawWidgetTypeRow holds raw column values before domain validation.
// Field order matches selectWidgetTypeColumns.
type rawWidgetTypeRow struct {
	id             uuid.UUID
	libraryID      uuid.UUID
	name           string
	htmlTemplate   string
	script         string
	language       string
	defaultWidth   int
	defaultHeight  int
	inputPortsJSON []byte
	version        int
}

func (r *widgetTypeRepositoryPostgresImpl) scanWidgetType(scan func(...any) error) (library.WidgetType, error) {
	var row rawWidgetTypeRow
	if err := scan(
		&row.id, &row.libraryID, &row.name, &row.htmlTemplate, &row.script, &row.language,
		&row.defaultWidth, &row.defaultHeight,
		&row.inputPortsJSON,
		&row.version,
	); err != nil {
		return nil, err
	}
	return reconstructWidgetType(row)
}

func reconstructWidgetType(row rawWidgetTypeRow) (library.WidgetType, error) {
	newID := id.NewID(id.IDWithUUID[library.WidgetType](row.id))

	newName, err := library.NewWidgetTypeName(row.name)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget type name: %w", err)
	}

	newHtml, err := library.NewHtmlTemplate(row.htmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("cannot create html template: %w", err)
	}

	newScript, err := library.NewScript(row.script)
	if err != nil {
		return nil, fmt.Errorf("cannot create script: %w", err)
	}

	newLang, err := library.NewScriptLanguage(row.language)
	if err != nil {
		return nil, fmt.Errorf("cannot create script language: %w", err)
	}

	defaultSize, err := library.NewSize(row.defaultWidth, row.defaultHeight)
	if err != nil {
		return nil, fmt.Errorf("cannot create default size: %w", err)
	}

	inputPorts, err := unmarshalInputPorts(row.inputPortsJSON)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal input ports: %w", err)
	}

	newVersion, err := version.New[library.WidgetType](version.WithNumber[library.WidgetType](row.version))
	if err != nil {
		return nil, fmt.Errorf("cannot create widget type version: %w", err)
	}

	return library.ReconstituteWidgetType(newID, id.NewID(id.IDWithUUID[library.Library](row.libraryID)), newName, newHtml, newScript, newLang, defaultSize, inputPorts, newVersion)
}

// inputPortJSON is the on-disk representation of an InputPort.
type inputPortJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TypeHint    string `json:"type_hint"`
}

func marshalInputPorts(ports []library.InputPort) ([]byte, error) {
	rows := make([]inputPortJSON, len(ports))
	for i, p := range ports {
		rows[i] = inputPortJSON{
			Name:        p.Name().String(),
			Description: p.Description(),
			TypeHint:    p.TypeHint().String(),
		}
	}
	return json.Marshal(rows)
}

func unmarshalInputPorts(data []byte) ([]library.InputPort, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var rows []inputPortJSON
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("cannot decode input_ports JSON: %w", err)
	}
	ports := make([]library.InputPort, 0, len(rows))
	for _, row := range rows {
		name, err := library.NewInputPortNameFromStorage(row.Name)
		if err != nil {
			return nil, fmt.Errorf("invalid stored input port name %q: %w", row.Name, err)
		}
		typeHint, err := library.NewPortTypeHint(row.TypeHint)
		if err != nil {
			return nil, fmt.Errorf("invalid stored type hint %q: %w", row.TypeHint, err)
		}
		ports = append(ports, library.NewInputPort(name, row.Description, typeHint))
	}
	return ports, nil
}
