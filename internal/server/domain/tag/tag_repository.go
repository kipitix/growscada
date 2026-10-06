package tag

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

var (
	ErrTagNotFound = errors.New("tag not found")
	// ErrTagConflict is returned by Save and Delete when the stored version
	// does not match, indicating a concurrent modification.
	ErrTagConflict = errors.New("tag version conflict")
	// ErrTagNameTaken is returned by Save when another tag already has the same name.
	// A tag's name is its unique natural key (ADR 0003).
	ErrTagNameTaken = errors.New("tag name already taken")
)

// TagRepository - repository interface for storing and managing tags.
// Defines operations for getting the next identifier,
// saving a tag, and retrieving a tag by its identifier.
type TagRepository interface {
	// NextID returns a new unique identifier for a tag
	NextID() id.ID[Tag]

	// Save stores a tag and the events it recorded, in one transaction: a tag
	// of the initial version is inserted, any other replaces the stored one
	// if the stored version is still the tag's version. The version is raised
	// once per Save; the returned tag carries the new one and no events.
	// Returns ErrTagNotFound if the tag does not exist on update,
	// ErrTagConflict if the stored version does not match, ErrTagNameTaken if
	// another tag already has the same name, or an error on failure.
	Save(context.Context, Tag) (Tag, error)

	// FindByID returns a tag by its identifier.
	// Returns the tag and nil on success, or nil and an error if not found or on failure.
	FindByID(context.Context, id.ID[Tag]) (Tag, error)

	// FindByName returns a tag by its unique name.
	// Returns the tag and nil on success, ErrTagNotFound if no tag has that name, or an error on failure.
	FindByName(context.Context, TagName) (Tag, error)

	// Delete removes a tag, read at the version it carries, and stores the
	// events it recorded (see Tag.Delete), in one transaction.
	// Returns ErrTagNotFound if the tag does not exist, ErrTagConflict if the
	// stored version does not match, or an error on failure.
	Delete(context.Context, Tag) error

	// FindAll returns all tags, sorted by name, so the order is stable while
	// tag values change.
	// Returns a slice of tags and nil on success, or an empty slice and an error on failure.
	FindAll(context.Context) ([]Tag, error)
}
