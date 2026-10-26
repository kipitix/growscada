package tag

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

var (
	ErrTagNotFound = errors.New("tag not found")
	// ErrTagConflict is returned by Save when the stored version does not
	// match, indicating a concurrent modification.
	ErrTagConflict = errors.New("tag version conflict")
	// ErrTagNameTaken is returned by Save when another tag of the same Project
	// already has the name. Within its Project a tag's name is its unique
	// natural key (ADR 0003).
	ErrTagNameTaken = errors.New("tag name already taken")
)

// TagRepository - repository interface for storing and managing the Tag
// definitions of one Project (ADR 0006), in the Draft (ADR 0011): every read sees only the tags of that Project, so
// a tag of another one is not found, and saving or deleting a tag of another
// Project is a programming error.
type TagRepository interface {
	// NextID returns a new unique identifier for a tag
	NextID() id.ID[Tag]

	// Save stores a tag and the events it recorded, in one transaction: a tag
	// of the initial version is inserted, any other replaces the stored one
	// if the stored version is still the tag's version. The version is raised
	// once per Save; the returned tag carries the new one and no events.
	// Returns ErrTagNotFound if the tag does not exist on update,
	// ErrTagConflict if the stored version does not match, ErrTagNameTaken if
	// another tag of the Project already has the same name, or an error on
	// failure (a tag of another Project included).
	Save(context.Context, Tag) (Tag, error)

	// FindByID returns a tag by its identifier.
	// Returns the tag and nil on success, or nil and an error if not found or on failure.
	FindByID(context.Context, id.ID[Tag]) (Tag, error)

	// FindTypesByIDs returns the TagTypes of the tags with the given IDs, read
	// in one query; a tag that does not exist is left out.
	// Returns the types and nil on success, or nil and an error on failure.
	FindTypesByIDs(context.Context, []id.ID[Tag]) (map[id.ID[Tag]]TagType, error)

	// FindByName returns a tag by its name, unique within the Project.
	// Returns the tag and nil on success, ErrTagNotFound if no tag has that name, or an error on failure.
	FindByName(context.Context, TagName) (Tag, error)

	// Delete removes a tag, whatever version is stored, and stores the events
	// it recorded (see Tag.Delete), in one transaction. The version is not
	// checked: removing the tag does not depend on what it was read at.
	// Returns ErrTagNotFound if the tag does not exist, or an error on failure.
	Delete(context.Context, Tag) error

	// FindAll returns all tags, sorted by name, so the order is stable while
	// tag values change.
	// Returns a slice of tags and nil on success, or an empty slice and an error on failure.
	FindAll(context.Context) ([]Tag, error)

	// FindAllInLiveProject is FindAll that also checks, in the same query,
	// that the Project is live and of the Organization: for the list the UI
	// reloads on every event, which then needs no read of the Project of its
	// own. A Project not one of those has no tags: the list is empty, as an
	// empty Project's, which the caller tells apart.
	FindAllInLiveProject(context.Context, id.ID[organization.Organization]) ([]Tag, error)
}
