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

type widgetTypeRepositoryPostgresImpl struct {
	db    *sql.DB
	store aggregateStore[library.WidgetType]
}

var _ library.WidgetTypeRepository = (*widgetTypeRepositoryPostgresImpl)(nil)

func NewWidgetTypeRepositoryPostgres(aDb *sql.DB, opts ...Option) library.WidgetTypeRepository {
	o := newOptions(opts)
	return &widgetTypeRepositoryPostgresImpl{
		db: aDb,
		store: aggregateStore[library.WidgetType]{
			db: aDb, table: tableNameWidgetTypes,
			notFound: library.ErrWidgetTypeNotFound, conflict: library.ErrWidgetTypeConflict,
			onCommit: o.onCommit,
		},
	}
}

func (r widgetTypeRepositoryPostgresImpl) NextID() id.ID[library.WidgetType] {
	return id.NewID[library.WidgetType]()
}

func (r widgetTypeRepositoryPostgresImpl) Save(ctx context.Context, wt library.WidgetType) (library.WidgetType, error) {
	portsJSON, err := marshalInputPorts(wt.InputPorts())
	if err != nil {
		return nil, fmt.Errorf("cannot serialize input ports: %w", err)
	}

	// The row is read back so that the result is what a later read returns.
	var saved library.WidgetType
	_, err = r.store.save(ctx, wt.ID(), wt.Version(), wt.PendingEvents(), rowWrite[library.WidgetType]{
		insert: func(tx *sql.Tx, next version.Version[library.WidgetType]) error {
			row := tx.QueryRowContext(ctx,
				`INSERT INTO widget_types (id, name, html_template, script, script_language, default_width, default_height, input_ports, version)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				RETURNING `+selectWidgetTypeColumns,
				wt.ID().UUID(), wt.Name().String(), wt.HtmlTemplate().String(),
				wt.Script().String(), wt.ScriptLanguage().String(),
				wt.DefaultSize().Width(), wt.DefaultSize().Height(),
				portsJSON, next.Number(),
			)
			if saved, err = r.scanWidgetType(row.Scan); err != nil {
				return fmt.Errorf("cannot insert new widget type: %w", err)
			}
			return nil
		},
		update: func(tx *sql.Tx, current, next version.Version[library.WidgetType]) (bool, error) {
			row := tx.QueryRowContext(ctx,
				`UPDATE widget_types
				 SET name = $1, html_template = $2, script = $3, script_language = $4,
				     default_width = $5, default_height = $6, input_ports = $7, version = $8
				 WHERE id = $9 AND version = $10
				 RETURNING `+selectWidgetTypeColumns,
				wt.Name().String(), wt.HtmlTemplate().String(), wt.Script().String(),
				wt.ScriptLanguage().String(),
				wt.DefaultSize().Width(), wt.DefaultSize().Height(),
				portsJSON, next.Number(),
				wt.ID().UUID(), current.Number(),
			)
			saved, err = r.scanWidgetType(row.Scan)
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			if err != nil {
				return false, fmt.Errorf("cannot update widget type: %w", err)
			}
			return true, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

func (r widgetTypeRepositoryPostgresImpl) Delete(ctx context.Context, wt library.WidgetType) error {
	err := r.store.delete(ctx, wt.ID(), wt.Version(), wt.PendingEvents())
	if isForeignKeyViolation(err, constraintWidgetsTypeID) {
		return library.ErrWidgetTypeInUse
	}
	return err
}

const selectWidgetTypeColumns = `id, name, html_template, script, script_language, default_width, default_height, input_ports, version`

func (r widgetTypeRepositoryPostgresImpl) FindByID(ctx context.Context, anID id.ID[library.WidgetType]) (library.WidgetType, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+selectWidgetTypeColumns+" FROM widget_types WHERE id = $1",
		anID.UUID(),
	)
	wt, err := r.scanWidgetType(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, library.ErrWidgetTypeNotFound
		}
		return nil, fmt.Errorf("error scanning widget type: %w", err)
	}
	return wt, nil
}

func (r widgetTypeRepositoryPostgresImpl) FindAll(ctx context.Context) ([]library.WidgetType, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+selectWidgetTypeColumns+" FROM widget_types ORDER BY pk_id",
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
	name           string
	htmlTemplate   string
	script         string
	language       string
	defaultWidth   int
	defaultHeight  int
	inputPortsJSON []byte
	version        int
}

func (r widgetTypeRepositoryPostgresImpl) scanWidgetType(scan func(...any) error) (library.WidgetType, error) {
	var row rawWidgetTypeRow
	if err := scan(
		&row.id, &row.name, &row.htmlTemplate, &row.script, &row.language,
		&row.defaultWidth, &row.defaultHeight,
		&row.inputPortsJSON,
		&row.version,
	); err != nil {
		return nil, err
	}
	return r.reconstruct(row)
}

func (r widgetTypeRepositoryPostgresImpl) reconstruct(row rawWidgetTypeRow) (library.WidgetType, error) {
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

	return library.ReconstituteWidgetType(newID, newName, newHtml, newScript, newLang, defaultSize, inputPorts, newVersion)
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
