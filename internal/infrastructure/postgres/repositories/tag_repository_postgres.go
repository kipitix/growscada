package repositories

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/domain/tag"
)

// TagRepositoryPostgres implements the TagRepository interface for PostgreSQL.
// Holds a database connection.
type TagRepositoryPostgres struct {
	db *sql.DB
}

// Verify that TagRepositoryPostgres implements tag.TagRepository
var _ tag.TagRepository = (*TagRepositoryPostgres)(nil)

// NewTagRepositoryPostgres creates a new PostgreSQL tag repository instance.
// Accepts a database connection and returns a pointer to TagRepositoryPostgres.
func NewTagRepositoryPostgres(aDb *sql.DB) *TagRepositoryPostgres {
	return &TagRepositoryPostgres{db: aDb}
}

// NextID generates a new unique tag identifier.
// Uses uuid.New() to generate a UUID and converts it to TagID.
func (r TagRepositoryPostgres) NextID() tag.TagID {
	return tag.TagID(uuid.New())
}

// Save stores a tag in the database.
// Implements save logic with version checking (optimistic locking).
func (r TagRepositoryPostgres) Save(ctx context.Context, aTag tag.Tag) error {
	// If the tag is new, insert it into the database
	if aTag.Version() == tag.TagVersionInitial {
		// SQL for inserting a new tag
		sqlResult, err := r.db.ExecContext(ctx,
			`INSERT INTO tags (id, name, kind, value, quality, version)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			aTag.ID().UUID(), aTag.Name().String(), aTag.Kind().String(), aTag.Value().String(), aTag.Quality().String(), aTag.Version(),
		)
		// Handle error
		if err != nil {
			return fmt.Errorf("cannot insert new tag: %w", err)
		}
		// Check the number of affected rows
		if rowsAffected, _ := sqlResult.RowsAffected(); rowsAffected != 1 {
			return fmt.Errorf("expected 1 row affected on insert, got %d", rowsAffected)
		}

		return nil
	}

	// If the tag already exists, update it in the database
	if aTag.Version() > tag.TagVersionInitial {
		// SQL for updating the tag
		sqlResult, err := r.db.ExecContext(ctx,
			`UPDATE tags
			 SET name = $1, kind = $2, value = $3, quality = $4, version = $5, version = version + 1
			 WHERE id = $6 AND version = $7`,
			aTag.Name().String(), aTag.Kind().String(), aTag.Value().String(), aTag.Quality().String(), aTag.Version(), aTag.ID().UUID(), aTag.Version(),
		)
		// Handle error
		if err != nil {
			return fmt.Errorf("cannot update tag: %w", err)
		}
		// Check the number of affected rows
		if rowsAffected, _ := sqlResult.RowsAffected(); rowsAffected != 1 {
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
// Temporarily returns nil, nil — implementation in progress.
// Will implement tag retrieval from the database table in the future.
func (r TagRepositoryPostgres) FindByID(ctx context.Context, id tag.TagID) (tag.Tag, error) {
	return nil, nil
	/*
	   query := `SELECT id, status, total_cents, version, updated_at FROM orders WHERE id = $1`

	   row := r.db.QueryRowContext(ctx, query, id)


	   var order domain.Order
	   var statusStr string
	   err := row.Scan(&order.id, &statusStr, &order.totalCents, &order.version, &order.updatedAt)

	   	if err != nil {
	   		if errors.Is(err, sql.ErrNoRows) {
	   			return nil, domain.ErrOrderNotFound
	   		}
	   		return nil, err
	   	}

	   order.status = domain.Status(statusStr)
	   return &order, nil
	*/
}

func (r TagRepositoryPostgres) FindAll(ctx context.Context) ([]tag.Tag, error) {
	// SQL for selecting all tags
	query := `SELECT id, name, kind, value, quality, version FROM tags`
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
			kind    string
			value   string
			quality string
			version int
		)
		// Scan row
		err := rows.Scan(&uuid, &name, &kind, &value, &quality, &version)
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
		// Create new TagKind value object
		newKind, err := tag.NewTagKind(kind)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag kind: %w", err)
		}
		// Create new TagValue value object
		newValue, err := tag.NewTagValue(value, newKind)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag value: %w", err)
		}
		// Create new TagQuality value object
		newQuality, err := tag.NewTagQuality(quality)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag quality: %w", err)
		}
		// Create new Tag aggregate
		newTag, err := tag.NewTag(newID, newName, newKind, newValue, newQuality, version)
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
