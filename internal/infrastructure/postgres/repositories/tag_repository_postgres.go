package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/tag"
)

// tagRepositoryPostgresImpl implements the TagRepository interface for PostgreSQL.
// Holds a database connection.
type tagRepositoryPostgresImpl struct {
	db *sql.DB
}

// Verify that TagRepositoryPostgres implements tag.TagRepository
var _ tag.TagRepository = (*tagRepositoryPostgresImpl)(nil)

// NewTagRepositoryPostgres creates a new PostgreSQL tag repository instance.
// Accepts a database connection and returns a pointer to TagRepositoryPostgres.
func NewTagRepositoryPostgres(aDb *sql.DB) tag.TagRepository {
	return &tagRepositoryPostgresImpl{db: aDb}
}

// NextID generates a new unique tag identifier.
// Uses uuid.New() to generate a UUID and converts it to TagID.
func (r tagRepositoryPostgresImpl) NextID() tag.TagID {
	return tag.TagID(uuid.New())
}

// Save stores a tag in the database.
// Implements save logic with version checking (optimistic locking).
func (r tagRepositoryPostgresImpl) Save(ctx context.Context, aTag tag.Tag) error {
	// If the tag is new, insert it into the database
	if aTag.Version() == tag.TagVersionInitial {
		// SQL for inserting a new tag
		sqlResult, err := r.db.ExecContext(ctx,
			`INSERT INTO tags (id, name, type, value, quality, version)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			aTag.ID().UUID(), aTag.Name().String(), aTag.Type().String(), aTag.Value().String(), aTag.Quality().String(), tag.TagVersionCommitted,
		)
		// Handle error
		if err != nil {
			return fmt.Errorf("cannot insert new tag: %w", err)
		}
		// Check the number of affected rows
		// RowsAffected error is always nil for postgres
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on insert: %w", err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("expected 1 row affected on insert, got %d", rowsAffected)
		}

		// Increment the tag version
		aTag.IncrementVersion()

		return nil
	}

	// If the tag already exists, update it in the database
	if aTag.Version() > tag.TagVersionInitial {
		// SQL for updating the tag
		sqlResult, err := r.db.ExecContext(ctx,
			`UPDATE tags
			 SET name = $1, type = $2, value = $3, quality = $4, version = version + 1
			 WHERE id = $5 AND version = $6`,
			aTag.Name().String(), aTag.Type().String(), aTag.Value().String(), aTag.Quality().String(), aTag.ID().UUID(), aTag.Version(),
		)
		// Handle error
		if err != nil {
			return fmt.Errorf("cannot update tag: %w", err)
		}
		// Check the number of affected rows
		// RowsAffected error is always nil for postgres
		rowsAffected, err := sqlResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("cannot get rows affected on update: %w", err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("expected 1 row affected on update, got %d", rowsAffected)
		}
		// Increment the tag version
		aTag.IncrementVersion()

		return nil
	}

	// If the tag version is negative, return an error
	return fmt.Errorf("undefined behavior with version %d", aTag.Version())
}

// FindByID retrieves a tag from the database by its identifier.
func (r tagRepositoryPostgresImpl) FindByID(ctx context.Context, id tag.TagID) (tag.Tag, error) {
	query := `SELECT id, name, type, value, quality, version FROM tags WHERE id = $1`

	var (
		tagUUID uuid.UUID
		name    string
		tagType string
		value   string
		quality string
		version int
	)

	row := r.db.QueryRowContext(ctx, query, id.UUID())
	err := row.Scan(&tagUUID, &name, &tagType, &value, &quality, &version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, tag.ErrTagNotFound
		}
		return nil, fmt.Errorf("error scanning tag: %w", err)
	}

	newID := tag.NewTagID(tag.TagIDWithUUID(tagUUID))

	newName, err := tag.NewTagName(name)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag name: %w", err)
	}

	newType, err := tag.NewTagType(tagType)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag type: %w", err)
	}

	newValue, err := newType.NewTagValue(value)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag value: %w", err)
	}

	newQuality, err := tag.NewTagQuality(quality)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag quality: %w", err)
	}

	return tag.NewTag(newID, newName, newType, newValue, newQuality, tag.TagVersion(version))
}

// DeleteByID removes a tag from the database by its identifier and returns it.
func (r tagRepositoryPostgresImpl) DeleteByID(ctx context.Context, id tag.TagID) (tag.Tag, error) {
	query := `DELETE FROM tags WHERE id = $1 RETURNING id, name, type, value, quality, version`

	var (
		tagUUID uuid.UUID
		name    string
		tagType string
		value   string
		quality string
		version int
	)

	row := r.db.QueryRowContext(ctx, query, id.UUID())
	err := row.Scan(&tagUUID, &name, &tagType, &value, &quality, &version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, tag.ErrTagNotFound
		}
		return nil, fmt.Errorf("cannot delete tag: %w", err)
	}

	newID := tag.NewTagID(tag.TagIDWithUUID(tagUUID))

	newName, err := tag.NewTagName(name)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag name: %w", err)
	}

	newType, err := tag.NewTagType(tagType)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag type: %w", err)
	}

	newValue, err := newType.NewTagValue(value)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag value: %w", err)
	}

	newQuality, err := tag.NewTagQuality(quality)
	if err != nil {
		return nil, fmt.Errorf("cannot create tag quality: %w", err)
	}

	return tag.NewTag(newID, newName, newType, newValue, newQuality, tag.TagVersion(version))
}

// FindAll retrieves all tags from the database.
func (r tagRepositoryPostgresImpl) FindAll(ctx context.Context) ([]tag.Tag, error) {
	// SQL for selecting all tags
	query := `SELECT id, name, type, value, quality, version FROM tags`
	// Execute query and process results
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error querying tags: %w", err)
	}
	// Close rows when done
	defer rows.Close()
	// Initialize slice for storing tags
	var tags []tag.Tag
	// Iterate over rows
	for rows.Next() {
		var (
			uuid    uuid.UUID
			name    string
			tagType string
			value   string
			quality string
			version int
		)
		// Scan row
		err := rows.Scan(&uuid, &name, &tagType, &value, &quality, &version)
		if err != nil {
			return nil, fmt.Errorf("error scanning tag: %w", err)
		}
		// Create new TagID value object
		newID := tag.NewTagID(tag.TagIDWithUUID(uuid))
		// Create new TagName value object
		newName, err := tag.NewTagName(name)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag name: %w", err)
		}
		// Create new TagType value object
		newType, err := tag.NewTagType(tagType)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag type: %w", err)
		}
		// Create new TagValue value object
		newValue, err := newType.NewTagValue(value)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag value: %w", err)
		}
		// Create new TagQuality value object
		newQuality, err := tag.NewTagQuality(quality)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag quality: %w", err)
		}
		// Create new Tag aggregate
		newTag, err := tag.NewTag(newID, newName, newType, newValue, newQuality, tag.TagVersion(version))
		if err != nil {
			return nil, fmt.Errorf("cannot create tag: %w", err)
		}
		// Append tag to slice
		tags = append(tags, newTag)
	}
	// Check for errors after iteration
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over tags: %w", err)
	}
	// Return tags slice
	return tags, nil
}
