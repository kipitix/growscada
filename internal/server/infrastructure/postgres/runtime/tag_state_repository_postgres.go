// Package runtime holds the Postgres repositories of the Runtime node
// (ADR 0011): the tables of the runtime schema, and reads of the server's
// public one (projects). It never queries the engineering schema.
package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/tagstate"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// tagStateRepositoryPostgres is the tagstate.Repository over runtime.tags:
// one row per Tag, its definition's copy beside its state. A write takes the
// next number of runtime.tag_state_seq; it is no change of an aggregate edited
// by people, so it writes no outbox row (ADR 0011).
type tagStateRepositoryPostgres struct {
	db *sql.DB
}

var _ tagstate.Repository = (*tagStateRepositoryPostgres)(nil)

// NewTagStateRepositoryPostgres returns the repository of the Tag states.
func NewTagStateRepositoryPostgres(aDb *sql.DB) tagstate.Repository {
	return &tagStateRepositoryPostgres{db: aDb}
}

const selectTagStateColumns = `id, name, type, version, value, quality, seq`

func (r tagStateRepositoryPostgres) FindInLiveProject(ctx context.Context, projectID id.ID[project.Project], tagID id.ID[tag.Tag], orgID id.ID[organization.Organization]) (tagstate.TagState, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT t.id, t.name, t.type, t.version, t.value, t.quality, t.seq
		 FROM runtime.tags t JOIN projects p ON p.id = t.project_id
		 WHERE t.id = $1 AND t.project_id = $2 AND p.deleted_at IS NULL AND p.organization_id = $3`,
		tagID.UUID(), projectID.UUID(), orgID.UUID(),
	)
	found, err := scanTagState(projectID, row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return tagstate.TagState{}, id.NotFound(tag.ErrTagNotFound, tagID)
	}
	if err != nil {
		return tagstate.TagState{}, fmt.Errorf("error scanning tag state: %w", err)
	}
	return found, nil
}

func (r tagStateRepositoryPostgres) FindByIDs(ctx context.Context, projectID id.ID[project.Project], tagIDs []id.ID[tag.Tag]) (map[id.ID[tag.Tag]]tagstate.TagState, error) {
	states := make(map[id.ID[tag.Tag]]tagstate.TagState, len(tagIDs))
	if len(tagIDs) == 0 {
		return states, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+selectTagStateColumns+` FROM runtime.tags WHERE id = ANY($1::uuid[]) AND project_id = $2`,
		pq.Array(rawIDs(tagIDs)), projectID.UUID(),
	)
	if err != nil {
		return nil, fmt.Errorf("error querying tag states: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		s, err := scanTagState(projectID, rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("error scanning tag state: %w", err)
		}
		states[s.TagID()] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over tag states: %w", err)
	}
	return states, nil
}

func (r tagStateRepositoryPostgres) IsProjectLive(ctx context.Context, projectID id.ID[project.Project], orgID id.ID[organization.Organization]) (bool, error) {
	var live bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL)`,
		projectID.UUID(), orgID.UUID(),
	).Scan(&live)
	if err != nil {
		return false, fmt.Errorf("cannot check project %s: %w", projectID, err)
	}
	return live, nil
}

// Write numbers the state in the UPDATE itself: concurrent writes of one Tag
// take their numbers in the order the row lock serializes them, so the state
// stored is the one with the greatest Seq.
func (r tagStateRepositoryPostgres) Write(ctx context.Context, aState tagstate.TagState) (tagstate.TagState, error) {
	var seq int64
	err := r.db.QueryRowContext(ctx,
		`UPDATE runtime.tags SET value = $1, quality = $2, seq = nextval('runtime.tag_state_seq')
		 WHERE id = $3 AND project_id = $4 AND type = $5
		 RETURNING seq`,
		aState.Value().String(), aState.Quality().String(),
		aState.TagID().UUID(), aState.ProjectID().UUID(), aState.Definition().Type.String(),
	).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return tagstate.TagState{}, id.NotFound(tag.ErrTagNotFound, aState.TagID())
	}
	if err != nil {
		return tagstate.TagState{}, fmt.Errorf("cannot write tag state: %w", err)
	}
	return tagstate.Reconstitute(aState.Definition(), aState.Value(), aState.Quality(), tagstate.Seq(seq))
}

// UpsertDefinitions inserts the missing rows in one transaction; a row
// already there keeps its state and takes the definition given.
func (r tagStateRepositoryPostgres) UpsertDefinitions(ctx context.Context, projectID id.ID[project.Project], defs []tagstate.Definition) error {
	if len(defs) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, def := range defs {
		if def.ProjectID != projectID {
			return fmt.Errorf("cannot upsert tag %s of project %s into project %s", def.TagID, def.ProjectID, projectID)
		}
		initial, err := tagstate.Initial(def)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO runtime.tags (id, project_id, name, type, version, value, quality)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, version = EXCLUDED.version
			 WHERE runtime.tags.project_id = EXCLUDED.project_id AND runtime.tags.type = EXCLUDED.type`,
			def.TagID.UUID(), projectID.UUID(), def.Name.String(), def.Type.String(), def.Version.Number(),
			initial.Value().String(), initial.Quality().String(),
		); err != nil {
			return fmt.Errorf("cannot upsert tag definition %s: %w", def.TagID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit tag definitions: %w", err)
	}
	return nil
}

func (r tagStateRepositoryPostgres) Delete(ctx context.Context, projectID id.ID[project.Project], tagIDs []id.ID[tag.Tag]) error {
	if len(tagIDs) == 0 {
		return nil
	}
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM runtime.tags WHERE id = ANY($1::uuid[]) AND project_id = $2`,
		pq.Array(rawIDs(tagIDs)), projectID.UUID(),
	); err != nil {
		return fmt.Errorf("cannot delete tag states: %w", err)
	}
	return nil
}

func (r tagStateRepositoryPostgres) DeleteAllOfProject(ctx context.Context, projectID id.ID[project.Project]) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM runtime.tags WHERE project_id = $1`, projectID.UUID()); err != nil {
		return fmt.Errorf("cannot delete tag states of project %s: %w", projectID, err)
	}
	return nil
}

// scanTagState reads a row of selectTagStateColumns of the Project.
func scanTagState(projectID id.ID[project.Project], scan func(...any) error) (tagstate.TagState, error) {
	var (
		tagUUID              uuid.UUID
		rawName, rawType     string
		rawVersion           int
		rawValue, rawQuality string
		seq                  int64
	)
	if err := scan(&tagUUID, &rawName, &rawType, &rawVersion, &rawValue, &rawQuality, &seq); err != nil {
		return tagstate.TagState{}, err
	}
	name, err := tag.NewTagName(rawName)
	if err != nil {
		return tagstate.TagState{}, fmt.Errorf("cannot create tag name: %w", err)
	}
	tagType, err := tag.NewTagType(rawType)
	if err != nil {
		return tagstate.TagState{}, fmt.Errorf("cannot create tag type: %w", err)
	}
	ver, err := version.New(version.WithNumber[tag.Tag](rawVersion))
	if err != nil {
		return tagstate.TagState{}, fmt.Errorf("cannot create tag version: %w", err)
	}
	value, err := tagType.NewTagValue(rawValue)
	if err != nil {
		return tagstate.TagState{}, fmt.Errorf("cannot create tag value: %w", err)
	}
	quality, err := tag.NewTagQuality(rawQuality)
	if err != nil {
		return tagstate.TagState{}, fmt.Errorf("cannot create tag quality: %w", err)
	}
	def := tagstate.Definition{TagID: id.NewID(id.IDWithUUID[tag.Tag](tagUUID)), ProjectID: projectID, Name: name, Type: tagType, Version: ver}
	return tagstate.Reconstitute(def, value, quality, tagstate.Seq(seq))
}

func rawIDs(tagIDs []id.ID[tag.Tag]) []string {
	raw := make([]string, len(tagIDs))
	for i, tagID := range tagIDs {
		raw[i] = tagID.UUID().String()
	}
	return raw
}
