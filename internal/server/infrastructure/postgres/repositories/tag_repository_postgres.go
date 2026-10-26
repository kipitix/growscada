package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/lib/pq"
)

// tagRepositoryPostgresImpl is the TagRepository of one Project, over the
// Tag definitions of the Draft (engineering.tags, ADR 0011): every query
// names project_id.
type tagRepositoryPostgresImpl struct {
	db      *sql.DB
	project id.ID[project.Project]
	store   aggregateStore[tag.Tag]
}

var _ tag.TagRepository = (*tagRepositoryPostgresImpl)(nil)

// NewTagRepositoryPostgres returns the repository of the tags of the Project
// aProjectID (ProjectScopes.ForProject is how the server opens one).
func NewTagRepositoryPostgres(aDb *sql.DB, aProjectID id.ID[project.Project], opts ...Option) tag.TagRepository {
	o := newOptions(opts)
	return &tagRepositoryPostgresImpl{
		db:      aDb,
		project: aProjectID,
		store: aggregateStore[tag.Tag]{
			db: aDb, table: tableNameTags, project: &aProjectID,
			notFound: tag.ErrTagNotFound, conflict: tag.ErrTagConflict,
			onCommit: o.onCommit,
		},
	}
}

func (r tagRepositoryPostgresImpl) NextID() id.ID[tag.Tag] {
	return id.NewID[tag.Tag]()
}

func (r tagRepositoryPostgresImpl) Save(ctx context.Context, aTag tag.Tag) (tag.Tag, error) {
	if err := r.store.checkProject(aTag.ProjectID()); err != nil {
		return nil, err
	}
	saved, err := r.store.save(ctx, aTag.ID(), aTag.Version(), aTag.PendingEvents(), rowWrite[tag.Tag]{
		insert: func(tx *sql.Tx, next version.Version[tag.Tag]) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO engineering.tags (id, project_id, name, type, version)
				VALUES ($1, $2, $3, $4, $5)`,
				aTag.ID().UUID(), r.project.UUID(), aTag.Name().String(), aTag.Type().String(), next.Number(),
			)
			return r.tagWriteError("insert new tag", err)
		},
		update: func(tx *sql.Tx, current, next version.Version[tag.Tag]) (bool, error) {
			res, err := tx.ExecContext(ctx,
				`UPDATE engineering.tags
				 SET name = $1, type = $2, version = $3
				 WHERE id = $4 AND version = $5 AND project_id = $6`,
				aTag.Name().String(), aTag.Type().String(), next.Number(), aTag.ID().UUID(), current.Number(), r.project.UUID(),
			)
			if err != nil {
				return false, r.tagWriteError("update tag", err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return false, fmt.Errorf("cannot get rows affected on update: %w", err)
			}
			return n == 1, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return tag.ReconstituteTag(aTag.ID(), r.project, aTag.Name(), aTag.Type(), saved)
}

// tagWriteError maps a failed write of a tags row: a name taken in the
// Project is ErrTagNameTaken.
func (r tagRepositoryPostgresImpl) tagWriteError(what string, err error) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err, constraintTagsName) {
		return tag.ErrTagNameTaken
	}
	return fmt.Errorf("cannot %s: %w", what, err)
}

// Delete removes the tag whatever its stored Version: removing does not
// depend on it.
func (r tagRepositoryPostgresImpl) Delete(ctx context.Context, aTag tag.Tag) error {
	if err := r.store.checkProject(aTag.ProjectID()); err != nil {
		return err
	}
	return r.store.deleteAnyVersion(ctx, aTag.ID(), aTag.PendingEvents())
}

func (r tagRepositoryPostgresImpl) FindByID(ctx context.Context, tagID id.ID[tag.Tag]) (tag.Tag, error) {
	found, err := r.scanTag(r.db.QueryRowContext(ctx,
		`SELECT id, name, type, version FROM engineering.tags WHERE id = $1 AND project_id = $2`,
		tagID.UUID(), r.project.UUID(),
	))
	if errors.Is(err, tag.ErrTagNotFound) {
		return nil, id.NotFound(tag.ErrTagNotFound, tagID)
	}
	return found, err
}

func (r tagRepositoryPostgresImpl) FindTypesByIDs(ctx context.Context, tagIDs []id.ID[tag.Tag]) (map[id.ID[tag.Tag]]tag.TagType, error) {
	types := make(map[id.ID[tag.Tag]]tag.TagType, len(tagIDs))
	if len(tagIDs) == 0 {
		return types, nil
	}
	rawIDs := make([]string, len(tagIDs))
	for i, tagID := range tagIDs {
		rawIDs[i] = tagID.UUID().String()
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, type FROM engineering.tags WHERE id = ANY($1::uuid[]) AND project_id = $2`,
		pq.Array(rawIDs), r.project.UUID())
	if err != nil {
		return nil, fmt.Errorf("error querying tag types: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			tagUUID uuid.UUID
			rawType string
		)
		if err := rows.Scan(&tagUUID, &rawType); err != nil {
			return nil, fmt.Errorf("error scanning tag type: %w", err)
		}
		tagType, err := tag.NewTagType(rawType)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag type: %w", err)
		}
		types[id.NewID(id.IDWithUUID[tag.Tag](tagUUID))] = tagType
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over tag types: %w", err)
	}
	return types, nil
}

func (r tagRepositoryPostgresImpl) FindByName(ctx context.Context, aName tag.TagName) (tag.Tag, error) {
	return r.scanTag(r.db.QueryRowContext(ctx,
		`SELECT id, name, type, version FROM engineering.tags WHERE name = $1 AND project_id = $2`,
		aName.String(), r.project.UUID(),
	))
}

// scanTag reads a single `id, name, type, version` row; no row is tag.ErrTagNotFound.
func (r tagRepositoryPostgresImpl) scanTag(row *sql.Row) (tag.Tag, error) {
	var (
		tagUUID uuid.UUID
		name    string
		tagType string
		ver     int
	)

	err := row.Scan(&tagUUID, &name, &tagType, &ver)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, tag.ErrTagNotFound
		}
		return nil, fmt.Errorf("error scanning tag: %w", err)
	}

	return r.reconstruct(tagUUID, name, tagType, ver)
}

func (r tagRepositoryPostgresImpl) FindAll(ctx context.Context) ([]tag.Tag, error) {
	return r.readTags(ctx,
		`SELECT id, name, type, version FROM engineering.tags WHERE project_id = $1 ORDER BY name`,
		r.project.UUID(),
	)
}

func (r tagRepositoryPostgresImpl) FindAllInLiveProject(ctx context.Context, orgID id.ID[organization.Organization]) ([]tag.Tag, error) {
	return r.readTags(ctx,
		`SELECT t.id, t.name, t.type, t.version
		 FROM engineering.tags t JOIN projects p ON p.id = t.project_id
		 WHERE t.project_id = $1 AND p.deleted_at IS NULL AND p.organization_id = $2
		 ORDER BY t.name`,
		r.project.UUID(), orgID.UUID(),
	)
}

// readTags reads the tags query selects, its columns those of FindAll.
func (r tagRepositoryPostgresImpl) readTags(ctx context.Context, query string, args ...any) ([]tag.Tag, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying tags: %w", err)
	}
	defer rows.Close()

	var tags []tag.Tag
	for rows.Next() {
		var (
			tagUUID    uuid.UUID
			tagName    string
			tagType    string
			tagVersion int
		)
		if err := rows.Scan(&tagUUID, &tagName, &tagType, &tagVersion); err != nil {
			return nil, fmt.Errorf("error scanning tag: %w", err)
		}
		t, err := r.reconstruct(tagUUID, tagName, tagType, tagVersion)
		if err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over tags: %w", err)
	}
	return tags, nil
}

func (r tagRepositoryPostgresImpl) reconstruct(aTagUUID uuid.UUID, aName, aTagType string, aVersion int) (tag.Tag, error) {
	newID := id.NewID(id.IDWithUUID[tag.Tag](aTagUUID))

	newName, err := tag.NewTagName(aName)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag name: %w", err)
	}

	newType, err := tag.NewTagType(aTagType)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag type: %w", err)
	}

	newVersion, err := version.New[tag.Tag](version.WithNumber[tag.Tag](aVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create tag version: %w", err)
	}

	return tag.ReconstituteTag(newID, r.project, newName, newType, newVersion)
}
