package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/tag"
	"github.com/kipitix/growscada/internal/domain/version"
)

type tagRepositoryPostgresImpl struct {
	db *sql.DB
}

var _ tag.TagRepository = (*tagRepositoryPostgresImpl)(nil)

func NewTagRepositoryPostgres(aDb *sql.DB) tag.TagRepository {
	return &tagRepositoryPostgresImpl{db: aDb}
}

func (r tagRepositoryPostgresImpl) NextID() id.ID[tag.Tag] {
	newID, _ := id.NewID[tag.Tag]()
	return newID
}

func (r tagRepositoryPostgresImpl) Save(ctx context.Context, aTag tag.Tag) (tag.Tag, error) {
	if aTag.Version() == version.Initial[tag.Tag]() {
		sqlResult, err := r.db.ExecContext(ctx,
			`INSERT INTO tags (id, name, type, value, quality, version)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			aTag.ID().UUID(), aTag.Name().String(), aTag.Type().String(),
			aTag.Value().String(), aTag.Quality().String(), version.Committed[tag.Tag]().Number(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot insert new tag: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on insert: %w", err)
		}
		if rowsAffected != 1 {
			return nil, fmt.Errorf("expected 1 row affected on insert, got %d", rowsAffected)
		}
		saved, err := tag.NewTag(aTag.ID(), aTag.Name(), aTag.Type(), aTag.Value(), aTag.Quality(), version.Committed[tag.Tag]())
		if err != nil {
			return nil, fmt.Errorf("cannot build saved tag: %w", err)
		}
		return saved, nil
	}

	if aTag.Version().IsCommitted() {
		sqlResult, err := r.db.ExecContext(ctx,
			`UPDATE tags
			 SET name = $1, type = $2, value = $3, quality = $4, version = version + 1
			 WHERE id = $5 AND version = $6`,
			aTag.Name().String(), aTag.Type().String(), aTag.Value().String(),
			aTag.Quality().String(), aTag.ID().UUID(), aTag.Version().Number(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot update tag: %w", err)
		}
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot get rows affected on update: %w", err)
		}
		if rowsAffected != 1 {
			return nil, fmt.Errorf("expected 1 row affected on update, got %d", rowsAffected)
		}
		saved, err := tag.NewTag(aTag.ID(), aTag.Name(), aTag.Type(), aTag.Value(), aTag.Quality(), aTag.Version().Next())
		if err != nil {
			return nil, fmt.Errorf("cannot build saved tag: %w", err)
		}
		return saved, nil
	}

	return nil, fmt.Errorf("undefined behavior with version %d", aTag.Version())
}

func (r tagRepositoryPostgresImpl) FindByID(ctx context.Context, tagID id.ID[tag.Tag]) (tag.Tag, error) {
	var (
		tagUUID uuid.UUID
		name    string
		tagType string
		value   string
		quality string
		ver     int
	)

	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, type, value, quality, version FROM tags WHERE id = $1`,
		tagID.UUID(),
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

func (r tagRepositoryPostgresImpl) DeleteByID(ctx context.Context, tagID id.ID[tag.Tag]) (tag.Tag, error) {
	var (
		tagUUID    uuid.UUID
		tagName    string
		tagType    string
		tagValue   string
		tagQuality string
		tagVersion int
	)

	row := r.db.QueryRowContext(ctx,
		`DELETE FROM tags WHERE id = $1 RETURNING id, name, type, value, quality, version`,
		tagID.UUID(),
	)
	err := row.Scan(&tagUUID, &tagName, &tagType, &tagValue, &tagQuality, &tagVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, tag.ErrTagNotFound
		}
		return nil, fmt.Errorf("cannot delete tag: %w", err)
	}

	return r.reconstruct(tagUUID, tagName, tagType, tagValue, tagQuality, tagVersion)
}

func (r tagRepositoryPostgresImpl) FindAll(ctx context.Context) ([]tag.Tag, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, type, value, quality, version FROM tags`,
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
	newID, _ := id.NewID(id.IDWithUUID[tag.Tag](aTagUUID))

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

	return tag.NewTag(newID, newName, newType, newValue, newQuality, newVersion)
}
