package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

type tagRepositoryPostgresImpl struct {
	db    *sql.DB
	store aggregateStore[tag.Tag]
}

var _ tag.TagRepository = (*tagRepositoryPostgresImpl)(nil)

func NewTagRepositoryPostgres(aDb *sql.DB, opts ...Option) tag.TagRepository {
	o := newOptions(opts)
	return &tagRepositoryPostgresImpl{
		db: aDb,
		store: aggregateStore[tag.Tag]{
			db: aDb, table: tableNameTags,
			notFound: tag.ErrTagNotFound, conflict: tag.ErrTagConflict,
			onCommit: o.onCommit,
		},
	}
}

func (r tagRepositoryPostgresImpl) NextID() id.ID[tag.Tag] {
	return id.NewID[tag.Tag]()
}

func (r tagRepositoryPostgresImpl) Save(ctx context.Context, aTag tag.Tag) (tag.Tag, error) {
	saved, err := r.store.save(ctx, aTag.ID(), aTag.Version(), aTag.PendingEvents(), rowWrite[tag.Tag]{
		insert: func(tx *sql.Tx, next version.Version[tag.Tag]) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO tags (id, name, type, value, quality, version)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				aTag.ID().UUID(), aTag.Name().String(), aTag.Type().String(),
				aTag.Value().String(), aTag.Quality().String(), next.Number(),
			)
			return tagWriteError("insert new tag", err)
		},
		update: func(tx *sql.Tx, current, next version.Version[tag.Tag]) (bool, error) {
			res, err := tx.ExecContext(ctx,
				`UPDATE tags
				 SET name = $1, type = $2, value = $3, quality = $4, version = $5
				 WHERE id = $6 AND version = $7`,
				aTag.Name().String(), aTag.Type().String(), aTag.Value().String(),
				aTag.Quality().String(), next.Number(), aTag.ID().UUID(), current.Number(),
			)
			if err != nil {
				return false, tagWriteError("update tag", err)
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
	return tag.ReconstituteTag(aTag.ID(), aTag.Name(), aTag.Type(), aTag.Value(), aTag.Quality(), saved)
}

// tagWriteError maps a failed write of a tags row: a taken name is
// ErrTagNameTaken.
func tagWriteError(what string, err error) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err, constraintTagsName) {
		return tag.ErrTagNameTaken
	}
	return fmt.Errorf("cannot %s: %w", what, err)
}

// Delete removes the tag whatever its stored Version: only the value changes
// after creation, and removing does not depend on it.
func (r tagRepositoryPostgresImpl) Delete(ctx context.Context, aTag tag.Tag) error {
	return r.store.deleteAnyVersion(ctx, aTag.ID(), aTag.PendingEvents())
}

func (r tagRepositoryPostgresImpl) FindByID(ctx context.Context, tagID id.ID[tag.Tag]) (tag.Tag, error) {
	return r.scanTag(r.db.QueryRowContext(ctx,
		`SELECT id, name, type, value, quality, version FROM tags WHERE id = $1`,
		tagID.UUID(),
	))
}

func (r tagRepositoryPostgresImpl) FindByName(ctx context.Context, aName tag.TagName) (tag.Tag, error) {
	return r.scanTag(r.db.QueryRowContext(ctx,
		`SELECT id, name, type, value, quality, version FROM tags WHERE name = $1`,
		aName.String(),
	))
}

// scanTag reads a single `id, name, type, value, quality, version` row; no row is tag.ErrTagNotFound.
func (r tagRepositoryPostgresImpl) scanTag(row *sql.Row) (tag.Tag, error) {
	var (
		tagUUID uuid.UUID
		name    string
		tagType string
		value   string
		quality string
		ver     int
	)

	err := row.Scan(&tagUUID, &name, &tagType, &value, &quality, &ver)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, tag.ErrTagNotFound
		}
		return nil, fmt.Errorf("error scanning tag: %w", err)
	}

	return r.reconstruct(tagUUID, name, tagType, value, quality, ver)
}

func (r tagRepositoryPostgresImpl) FindAll(ctx context.Context) ([]tag.Tag, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, type, value, quality, version FROM tags ORDER BY name`,
	)
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
			tagValue   string
			tagQuality string
			tagVersion int
		)
		if err := rows.Scan(&tagUUID, &tagName, &tagType, &tagValue, &tagQuality, &tagVersion); err != nil {
			return nil, fmt.Errorf("error scanning tag: %w", err)
		}
		t, err := r.reconstruct(tagUUID, tagName, tagType, tagValue, tagQuality, tagVersion)
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

func (r tagRepositoryPostgresImpl) reconstruct(
	aTagUUID uuid.UUID, aName, aTagType, aValue, aQuality string, aVersion int,
) (tag.Tag, error) {
	newID := id.NewID(id.IDWithUUID[tag.Tag](aTagUUID))

	newName, err := tag.NewTagName(aName)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag name: %w", err)
	}

	newType, err := tag.NewTagType(aTagType)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag type: %w", err)
	}

	newValue, err := newType.NewTagValue(aValue)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag value: %w", err)
	}

	newQuality, err := tag.NewTagQuality(aQuality)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag quality: %w", err)
	}

	newVersion, err := version.New[tag.Tag](version.WithNumber[tag.Tag](aVersion))
	if err != nil {
		return nil, fmt.Errorf("cannot create tag version: %w", err)
	}

	return tag.ReconstituteTag(newID, newName, newType, newValue, newQuality, newVersion)
}
