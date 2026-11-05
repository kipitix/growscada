package tag

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

var (
	ErrTagNotFound = errors.New("tag not found")
	// ErrTagNameTaken is returned by Save when another tag of the same Project
	// already has the name. Within its Project a tag's name is its unique
	// natural key (ADR 0003).
	ErrTagNameTaken = errors.New("tag name already taken")
	// ErrSupplierOfAnotherProject is returned by Save when the Tag's supplier
	// is not a Device of the Tag's Project (ADR 0006).
	ErrSupplierOfAnotherProject = errors.New("tag supplier of another project")
	// ErrTagNotSuppliedByDevice is returned by the Runtime node's write of a
	// state when the writer is not the Device that supplies the Tag: the Tag
	// exists, but only its supplier writes its values.
	ErrTagNotSuppliedByDevice = errors.New("tag is not supplied by the device")
)

// TagRepository reads the Tag definitions of one Project (ADR 0006), in the
// Draft (ADR 0011): every read sees only the tags of that Project. A Tag
// definition is an entity of the Project's Draft (ADR 0016): its writes go
// through the Draft's repository, never through here.
type TagRepository interface {
	// NextID returns a new unique identifier for a tag
	NextID() id.ID[Tag]

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
