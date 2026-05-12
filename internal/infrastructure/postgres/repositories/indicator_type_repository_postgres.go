package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/indicator_type"
)

type indicatorTypeRepositoryPostgresImpl struct {
	db *sql.DB
}

var _ indicator_type.IndicatorTypeRepository = (*indicatorTypeRepositoryPostgresImpl)(nil)

func NewIndicatorTypeRepositoryPostgres(aDb *sql.DB) indicator_type.IndicatorTypeRepository {
	return &indicatorTypeRepositoryPostgresImpl{db: aDb}
}

func (r indicatorTypeRepositoryPostgresImpl) NextID() indicator_type.IndicatorTypeID {
	return indicator_type.NewIndicatorTypeID()
}

func (r indicatorTypeRepositoryPostgresImpl) Save(ctx context.Context, it indicator_type.IndicatorType) error {
	if it.Version() == indicator_type.IndicatorTypeVersionInitial {
		sqlResult, err := r.db.ExecContext(ctx,
			`INSERT INTO indicator_types (id, name, svg_template, script, script_language, version)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			it.ID().UUID(), it.Name().String(), it.SvgTemplate().String(),
			it.Script().String(), it.ScriptLanguage().String(),
			indicator_type.IndicatorTypeVersionCommitted.Number(),
		)
		if err != nil {
			return fmt.Errorf("cannot insert new indicator type: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on insert: %w", err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("expected 1 row affected on insert, got %d", rowsAffected)
		}
		it.IncrementVersion()
		return nil
	}

	if it.Version().IsCommitted() {
		sqlResult, err := r.db.ExecContext(ctx,
			`UPDATE indicator_types
			 SET name = $1, svg_template = $2, script = $3, script_language = $4, version = version + 1
			 WHERE id = $5 AND version = $6`,
			it.Name().String(), it.SvgTemplate().String(), it.Script().String(),
			it.ScriptLanguage().String(), it.ID().UUID(), it.Version().Number(),
		)
		if err != nil {
			return fmt.Errorf("cannot update indicator type: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on update: %w", err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("expected 1 row affected on update, got %d", rowsAffected)
		}
		it.IncrementVersion()
		return nil
	}

	return fmt.Errorf("undefined behavior with version %d", it.Version().Number())
}

func (r indicatorTypeRepositoryPostgresImpl) FindByID(ctx context.Context, id indicator_type.IndicatorTypeID) (indicator_type.IndicatorType, error) {
	var (
		rawID   uuid.UUID
		name    string
		svg     string
		script  string
		lang    string
		version int
	)

	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, svg_template, script, script_language, version FROM indicator_types WHERE id = $1`,
		id.UUID(),
	)
	err := row.Scan(&rawID, &name, &svg, &script, &lang, &version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, indicator_type.ErrIndicatorTypeNotFound
		}
		return nil, fmt.Errorf("error scanning indicator type: %w", err)
	}

	return r.reconstruct(rawID, name, svg, script, lang, version)
}

func (r indicatorTypeRepositoryPostgresImpl) DeleteByID(ctx context.Context, id indicator_type.IndicatorTypeID) (indicator_type.IndicatorType, error) {
	var (
		rawID   uuid.UUID
		name    string
		svg     string
		script  string
		lang    string
		version int
	)

	row := r.db.QueryRowContext(ctx,
		`DELETE FROM indicator_types WHERE id = $1 RETURNING id, name, svg_template, script, script_language, version`,
		id.UUID(),
	)
	err := row.Scan(&rawID, &name, &svg, &script, &lang, &version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, indicator_type.ErrIndicatorTypeNotFound
		}
		return nil, fmt.Errorf("cannot delete indicator type: %w", err)
	}

	return r.reconstruct(rawID, name, svg, script, lang, version)
}

func (r indicatorTypeRepositoryPostgresImpl) FindAll(ctx context.Context) ([]indicator_type.IndicatorType, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, svg_template, script, script_language, version FROM indicator_types`,
	)
	if err != nil {
		return nil, fmt.Errorf("error querying indicator types: %w", err)
	}
	defer rows.Close()

	var result []indicator_type.IndicatorType
	for rows.Next() {
		var (
			rawID   uuid.UUID
			name    string
			svg     string
			script  string
			lang    string
			version int
		)
		if err := rows.Scan(&rawID, &name, &svg, &script, &lang, &version); err != nil {
			return nil, fmt.Errorf("error scanning indicator type: %w", err)
		}
		it, err := r.reconstruct(rawID, name, svg, script, lang, version)
		if err != nil {
			return nil, err
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over indicator types: %w", err)
	}
	return result, nil
}

func (r indicatorTypeRepositoryPostgresImpl) reconstruct(
	rawID uuid.UUID, name, svg, script, lang string, version int,
) (indicator_type.IndicatorType, error) {
	newID := indicator_type.NewIndicatorTypeID(indicator_type.IndicatorTypeIDWithUUID(rawID))

	newName, err := indicator_type.NewIndicatorTypeName(name)
	if err != nil {
		return nil, fmt.Errorf("cannot create indicator type name: %w", err)
	}

	newSvg, err := indicator_type.NewSvgTemplate(svg)
	if err != nil {
		return nil, fmt.Errorf("cannot create svg template: %w", err)
	}

	newScript, err := indicator_type.NewScript(script)
	if err != nil {
		return nil, fmt.Errorf("cannot create script: %w", err)
	}

	newLang, err := indicator_type.NewScriptLanguage(lang)
	if err != nil {
		return nil, fmt.Errorf("cannot create script language: %w", err)
	}

	newVersion, err := indicator_type.NewIndicatorTypeVersion(indicator_type.IndicatorTypeVersionWithNumber(version))
	if err != nil {
		return nil, fmt.Errorf("cannot create indicator type version: %w", err)
	}

	return indicator_type.NewIndicatorType(newID, newName, newSvg, newScript, newLang, newVersion)
}
