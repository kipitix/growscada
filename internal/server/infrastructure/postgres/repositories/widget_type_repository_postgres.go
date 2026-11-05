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
)

// widgetTypeRepositoryPostgresImpl is the read repository of the WidgetTypes
// of one Library's Draft: every query names library_id, so a widget type of
// another Library is not found. The writes are the Draft's (ADR 0016).
type widgetTypeRepositoryPostgresImpl struct {
	db      *sql.DB
	library id.ID[library.Library]
}

var _ library.WidgetTypeRepository = (*widgetTypeRepositoryPostgresImpl)(nil)

// widgetTypeRepositories opens the WidgetTypeRepository of a Library's Draft.
type widgetTypeRepositories struct {
	db *sql.DB
}

var _ library.WidgetTypeRepositories = (*widgetTypeRepositories)(nil)

// NewWidgetTypeRepositories returns the WidgetTypeRepositories over the
// database.
func NewWidgetTypeRepositories(aDb *sql.DB) library.WidgetTypeRepositories {
	return &widgetTypeRepositories{db: aDb}
}

func (r *widgetTypeRepositories) ForLibrary(libraryID id.ID[library.Library]) library.WidgetTypeRepository {
	return NewWidgetTypeRepositoryPostgres(r.db, libraryID)
}

// NewWidgetTypeRepositoryPostgres returns the read repository of the
// WidgetTypes of the Library aLibraryID.
func NewWidgetTypeRepositoryPostgres(aDb *sql.DB, aLibraryID id.ID[library.Library]) library.WidgetTypeRepository {
	return &widgetTypeRepositoryPostgresImpl{db: aDb, library: aLibraryID}
}

func (r *widgetTypeRepositoryPostgresImpl) NextID() id.ID[library.WidgetType] {
	return id.NewID[library.WidgetType]()
}

const selectWidgetTypeColumns = `id, library_id, name, html_template, script, script_language, default_width, default_height, input_ports`

func (r *widgetTypeRepositoryPostgresImpl) FindByID(ctx context.Context, anID id.ID[library.WidgetType]) (library.WidgetType, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+selectWidgetTypeColumns+" FROM engineering.widget_types WHERE id = $1 AND library_id = $2",
		anID.UUID(), r.library.UUID(),
	)
	wt, err := scanWidgetType(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, id.NotFound(library.ErrWidgetTypeNotFound, anID)
		}
		return nil, fmt.Errorf("error scanning widget type: %w", err)
	}
	return wt, nil
}

func (r *widgetTypeRepositoryPostgresImpl) FindAll(ctx context.Context) ([]library.WidgetType, error) {
	return findWidgetTypes(ctx, r.db, r.library)
}

// queryer is what reads rows: a *sql.DB or a *sql.Tx.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// findWidgetTypes returns the widget types of the Library, in creation order,
// as q reads them.
func findWidgetTypes(ctx context.Context, q queryer, libraryID id.ID[library.Library]) ([]library.WidgetType, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT "+selectWidgetTypeColumns+" FROM engineering.widget_types WHERE library_id = $1 ORDER BY pk_id",
		libraryID.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widget types: %w", err)
	}
	defer rows.Close()

	var result []library.WidgetType
	for rows.Next() {
		wt, err := scanWidgetType(rows.Scan)
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
}

func scanWidgetType(scan func(...any) error) (library.WidgetType, error) {
	var row rawWidgetTypeRow
	if err := scan(
		&row.id, &row.libraryID, &row.name, &row.htmlTemplate, &row.script, &row.language,
		&row.defaultWidth, &row.defaultHeight,
		&row.inputPortsJSON,
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

	return library.ReconstituteWidgetType(newID, id.NewID(id.IDWithUUID[library.Library](row.libraryID)), newName, newHtml, newScript, newLang, defaultSize, inputPorts)
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
