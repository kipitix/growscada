package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/version"
	"github.com/kipitix/growscada/internal/domain/widget"
)

type widgetTypeRepositoryPostgresImpl struct {
	db *sql.DB
}

var _ widget.WidgetTypeRepository = (*widgetTypeRepositoryPostgresImpl)(nil)

func NewWidgetTypeRepositoryPostgres(aDb *sql.DB) widget.WidgetTypeRepository {
	return &widgetTypeRepositoryPostgresImpl{db: aDb}
}

func (r widgetTypeRepositoryPostgresImpl) NextID() id.ID[widget.WidgetType] {
	return id.NewID[widget.WidgetType]()
}

func (r widgetTypeRepositoryPostgresImpl) Save(ctx context.Context, wt widget.WidgetType) (widget.WidgetType, error) {
	portsJSON, err := marshalInputPorts(wt.InputPorts())
	if err != nil {
		return nil, fmt.Errorf("cannot serialize input ports: %w", err)
	}

	if wt.Version() == version.Initial[widget.WidgetType]() {
		row := r.db.QueryRowContext(ctx,
			`INSERT INTO widget_types (id, name, html_template, script, script_language, default_width, default_height, input_ports, version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING `+selectWidgetTypeColumns,
			wt.ID().UUID(), wt.Name().String(), wt.HtmlTemplate().String(),
			wt.Script().String(), wt.ScriptLanguage().String(),
			wt.DefaultSize().Width(), wt.DefaultSize().Height(),
			portsJSON,
			version.Committed[widget.WidgetType]().Number(),
		)
		saved, err := r.scanWidgetType(row.Scan)
		if err != nil {
			return nil, fmt.Errorf("cannot insert new widget type: %w", err)
		}
		return saved, nil
	}

	if wt.Version().IsCommitted() {
		row := r.db.QueryRowContext(ctx,
			`UPDATE widget_types
			 SET name = $1, html_template = $2, script = $3, script_language = $4,
			     default_width = $5, default_height = $6, input_ports = $7, version = version + 1
			 WHERE id = $8 AND version = $9
			 RETURNING `+selectWidgetTypeColumns,
			wt.Name().String(), wt.HtmlTemplate().String(), wt.Script().String(),
			wt.ScriptLanguage().String(),
			wt.DefaultSize().Width(), wt.DefaultSize().Height(),
			portsJSON,
			wt.ID().UUID(), wt.Version().Number(),
		)
		saved, err := r.scanWidgetType(row.Scan)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, classifyUpdateConflict(ctx, r.db, tableNameWidgetTypes, wt.ID(), widget.ErrWidgetTypeNotFound, widget.ErrWidgetTypeConflict)
			}
			return nil, fmt.Errorf("cannot update widget type: %w", err)
		}
		return saved, nil
	}

	return nil, fmt.Errorf("undefined behavior with version %d", wt.Version().Number())
}

const selectWidgetTypeColumns = `id, name, html_template, script, script_language, default_width, default_height, input_ports, version`

func (r widgetTypeRepositoryPostgresImpl) FindByID(ctx context.Context, anID id.ID[widget.WidgetType]) (widget.WidgetType, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+selectWidgetTypeColumns+" FROM widget_types WHERE id = $1",
		anID.UUID(),
	)
	wt, err := r.scanWidgetType(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetTypeNotFound
		}
		return nil, fmt.Errorf("error scanning widget type: %w", err)
	}
	return wt, nil
}

func (r widgetTypeRepositoryPostgresImpl) DeleteByID(ctx context.Context, anID id.ID[widget.WidgetType]) (widget.WidgetType, error) {
	row := r.db.QueryRowContext(ctx,
		"DELETE FROM widget_types WHERE id = $1 RETURNING "+selectWidgetTypeColumns,
		anID.UUID(),
	)
	wt, err := r.scanWidgetType(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetTypeNotFound
		}
		return nil, fmt.Errorf("cannot delete widget type: %w", err)
	}
	return wt, nil
}

func (r widgetTypeRepositoryPostgresImpl) FindAll(ctx context.Context) ([]widget.WidgetType, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+selectWidgetTypeColumns+" FROM widget_types",
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widget types: %w", err)
	}
	defer rows.Close()

	var result []widget.WidgetType
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

func (r widgetTypeRepositoryPostgresImpl) scanWidgetType(scan func(...any) error) (widget.WidgetType, error) {
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

func (r widgetTypeRepositoryPostgresImpl) reconstruct(row rawWidgetTypeRow) (widget.WidgetType, error) {
	newID := id.NewID(id.IDWithUUID[widget.WidgetType](row.id))

	newName, err := widget.NewWidgetTypeName(row.name)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget type name: %w", err)
	}

	newHtml, err := widget.NewHtmlTemplate(row.htmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("cannot create html template: %w", err)
	}

	newScript, err := widget.NewScript(row.script)
	if err != nil {
		return nil, fmt.Errorf("cannot create script: %w", err)
	}

	newLang, err := widget.NewScriptLanguage(row.language)
	if err != nil {
		return nil, fmt.Errorf("cannot create script language: %w", err)
	}

	defaultSize, err := widget.NewSize(row.defaultWidth, row.defaultHeight)
	if err != nil {
		return nil, fmt.Errorf("cannot create default size: %w", err)
	}

	inputPorts, err := unmarshalInputPorts(row.inputPortsJSON)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal input ports: %w", err)
	}

	newVersion, err := version.New[widget.WidgetType](version.WithNumber[widget.WidgetType](row.version))
	if err != nil {
		return nil, fmt.Errorf("cannot create widget type version: %w", err)
	}

	return widget.NewWidgetType(newID, newName, newHtml, newScript, newLang, defaultSize, inputPorts, newVersion)
}

// inputPortJSON is the on-disk representation of an InputPort.
type inputPortJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TypeHint    string `json:"type_hint"`
}

func marshalInputPorts(ports []widget.InputPort) ([]byte, error) {
	rows := make([]inputPortJSON, len(ports))
	for i, p := range ports {
		typeHint := ""
		if p.TypeHint() != tag.TagTypeUnknown {
			typeHint = p.TypeHint().String()
		}
		rows[i] = inputPortJSON{
			Name:        p.Name().String(),
			Description: p.Description(),
			TypeHint:    typeHint,
		}
	}
	return json.Marshal(rows)
}

func unmarshalInputPorts(data []byte) ([]widget.InputPort, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var rows []inputPortJSON
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("cannot decode input_ports JSON: %w", err)
	}
	ports := make([]widget.InputPort, 0, len(rows))
	for _, row := range rows {
		name, err := widget.NewInputPortNameFromStorage(row.Name)
		if err != nil {
			return nil, fmt.Errorf("invalid stored input port name %q: %w", row.Name, err)
		}
		typeHint, err := tag.NewTagType(row.TypeHint)
		if err != nil {
			return nil, fmt.Errorf("invalid stored type hint %q: %w", row.TypeHint, err)
		}
		ports = append(ports, widget.NewInputPort(name, row.Description, typeHint))
	}
	return ports, nil
}
