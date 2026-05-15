package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
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

func (r widgetTypeRepositoryPostgresImpl) NextID() widget.WidgetTypeID {
	return widget.NewWidgetTypeID()
}

func (r widgetTypeRepositoryPostgresImpl) Save(ctx context.Context, wt widget.WidgetType) (widget.WidgetType, error) {
	if wt.Version() == version.Initial {
		sqlResult, err := r.db.ExecContext(ctx,
			`INSERT INTO widget_types (id, name, html_template, script, script_language, version)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			wt.ID().UUID(), wt.Name().String(), wt.HtmlTemplate().String(),
			wt.Script().String(), wt.ScriptLanguage().String(), version.Committed.Number(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot insert new widget type: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on insert: %w", err)
		}
		if rowsAffected != 1 {
			return nil, fmt.Errorf("expected 1 row affected on insert, got %d", rowsAffected)
		}
		return widget.NewWidgetType(wt.ID(), wt.Name(), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), version.Committed), nil
	}

	if wt.Version().IsCommitted() {
		sqlResult, err := r.db.ExecContext(ctx,
			`UPDATE widget_types
			 SET name = $1, html_template = $2, script = $3, script_language = $4, version = version + 1
			 WHERE id = $5 AND version = $6`,
			wt.Name().String(), wt.HtmlTemplate().String(), wt.Script().String(),
			wt.ScriptLanguage().String(), wt.ID().UUID(), wt.Version().Number(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot update widget type: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on update: %w", err)
		}
		if rowsAffected != 1 {
			return nil, fmt.Errorf("expected 1 row affected on update, got %d", rowsAffected)
		}
		return widget.NewWidgetType(wt.ID(), wt.Name(), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.Version().Next()), nil
	}

	return nil, fmt.Errorf("undefined behavior with version %d", wt.Version().Number())
}

func (r widgetTypeRepositoryPostgresImpl) FindByID(ctx context.Context, id widget.WidgetTypeID) (widget.WidgetType, error) {
	var (
		rawID             uuid.UUID
		name              string
		htmlTemplate      string
		script            string
		language          string
		widgetTypeVersion int
	)

	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, html_template, script, script_language, version FROM widget_types WHERE id = $1`,
		id.UUID(),
	)
	err := row.Scan(&rawID, &name, &htmlTemplate, &script, &language, &widgetTypeVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetTypeNotFound
		}
		return nil, fmt.Errorf("error scanning widget type: %w", err)
	}

	return r.reconstruct(rawID, name, htmlTemplate, script, language, widgetTypeVersion)
}

func (r widgetTypeRepositoryPostgresImpl) DeleteByID(ctx context.Context, id widget.WidgetTypeID) (widget.WidgetType, error) {
	var (
		rawID             uuid.UUID
		name              string
		htmlTemplate      string
		script            string
		lang              string
		widgetTypeVersion int
	)

	row := r.db.QueryRowContext(ctx,
		`DELETE FROM widget_types WHERE id = $1 RETURNING id, name, html_template, script, script_language, version`,
		id.UUID(),
	)
	err := row.Scan(&rawID, &name, &htmlTemplate, &script, &lang, &widgetTypeVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, widget.ErrWidgetTypeNotFound
		}
		return nil, fmt.Errorf("cannot delete widget type: %w", err)
	}

	return r.reconstruct(rawID, name, htmlTemplate, script, lang, widgetTypeVersion)
}

func (r widgetTypeRepositoryPostgresImpl) FindAll(ctx context.Context) ([]widget.WidgetType, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, html_template, script, script_language, version FROM widget_types`,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying widget types: %w", err)
	}
	defer rows.Close()

	var result []widget.WidgetType
	for rows.Next() {
		var (
			rawID             uuid.UUID
			name              string
			htmlTemplate      string
			script            string
			lang              string
			widgetTypeVersion int
		)
		if err := rows.Scan(&rawID, &name, &htmlTemplate, &script, &lang, &widgetTypeVersion); err != nil {
			return nil, fmt.Errorf("error scanning widget type: %w", err)
		}
		wt, err := r.reconstruct(rawID, name, htmlTemplate, script, lang, widgetTypeVersion)
		if err != nil {
			return nil, err
		}
		result = append(result, wt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over widget types: %w", err)
	}
	return result, nil
}

func (r widgetTypeRepositoryPostgresImpl) reconstruct(
	aRawID uuid.UUID, aName, aHTMLTemplate, aScript, aLanguage string, aVersion int,
) (widget.WidgetType, error) {
	newID := widget.NewWidgetTypeID(widget.WidgetTypeIDWithUUID(aRawID))

	newName, err := widget.NewWidgetTypeName(aName)
	if err != nil {
		return nil, fmt.Errorf("cannot create widget type name: %w", err)
	}

	newHtml, err := widget.NewHtmlTemplate(aHTMLTemplate)
	if err != nil {
		return nil, fmt.Errorf("cannot create html template: %w", err)
	}

	newScript, err := widget.NewScript(aScript)
	if err != nil {
		return nil, fmt.Errorf("cannot create script: %w", err)
	}

	newLang, err := widget.NewScriptLanguage(aLanguage)
	if err != nil {
		return nil, fmt.Errorf("cannot create script language: %w", err)
	}

	newVersion, err := version.New(version.WithNumber(aVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create widget type version: %w", err)
	}

	return widget.NewWidgetType(newID, newName, newHtml, newScript, newLang, newVersion), nil
}
