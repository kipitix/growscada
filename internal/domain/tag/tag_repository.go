package tag

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/domain/id"
)

var (
	ErrTagNotFound = errors.New("tag not found")
	// ErrTagConflict is returned by Save when the stored version does not match,
	// indicating a concurrent modification.
	ErrTagConflict = errors.New("tag version conflict")
)

// TagRepository - repository interface for storing and managing tags.
// Defines operations for getting the next identifier,
// saving a tag, and retrieving a tag by its identifier.
type TagRepository interface {
	// NextID returns a new unique identifier for a tag
	NextID() id.ID[Tag]

	// Save stores a tag in the repository.
	// Returns the saved tag with updated version on success, or an error on failure.
	Save(context.Context, Tag) (Tag, error)

	// FindByID returns a tag by its identifier.
	// Returns the tag and nil on success, or nil and an error if not found or on failure.
	FindByID(context.Context, id.ID[Tag]) (Tag, error)

	// DeleteByID removes a tag by its identifier and returns it.
	// Returns the deleted tag and nil on success, ErrTagNotFound if the tag does not exist, or an error on failure.
	DeleteByID(context.Context, id.ID[Tag]) (Tag, error)

	// FindAll returns all tags.
	// Returns a slice of tags and nil on success, or an empty slice and an error on failure.
	FindAll(context.Context) ([]Tag, error)
}
