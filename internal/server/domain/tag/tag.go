package tag

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Tag - interface representing a Tag definition: the Tag's name and TagType,
// configuration of the Draft held by the Engineering node (ADR 0011). Its
// value and Quality are its Tag state, held by the Runtime node
// (tagstate.TagState). It belongs to exactly one Project, fixed at creation
// (ADR 0006).
// Immutable aggregate: Delete returns a new Tag.
type Tag interface {
	ID() id.ID[Tag]
	// ProjectID is the Project the tag belongs to; it never changes.
	ProjectID() id.ID[project.Project]
	Name() TagName
	Type() TagType
	Version() version.Version[Tag]

	// Delete returns the Tag with a TagDeletedEvent recorded; the
	// repository's Delete removes it.
	Delete() Tag

	event.Recorder
	fmt.Stringer
}

// tagImpl - tag implementation struct
type tagImpl struct {
	id        id.ID[Tag]
	projectID id.ID[project.Project]
	name      TagName
	tagType   TagType
	version   version.Version[Tag]
	pending   []event.Event
}

var _ Tag = (*tagImpl)(nil)

// CreateTag creates a new, not yet saved tag of the Project aProjectID and
// records a TagCreatedEvent. Its state, until a supplier writes one, is its
// TagType's zero value with Quality Bad (tagstate.Initial).
// Returns an error if the type is the invalid zero value.
func CreateTag(anID id.ID[Tag], aProjectID id.ID[project.Project], aName TagName, aType TagType) (Tag, error) {
	t, err := ReconstituteTag(anID, aProjectID, aName, aType, version.Initial[Tag]())
	if err != nil {
		return nil, err
	}
	impl := t.(*tagImpl)
	impl.pending = []event.Event{NewTagCreatedEvent(anID, aProjectID)}
	return impl, nil
}

// ReconstituteTag rebuilds a tag as stored, at the given version; it records
// no event. For repositories.
// Returns an error if the type is the invalid zero value.
func ReconstituteTag(anID id.ID[Tag], aProjectID id.ID[project.Project], aName TagName, aType TagType, aVersion version.Version[Tag]) (Tag, error) {
	if !aType.IsValid() {
		return nil, fmt.Errorf("cannot create tag: invalid type")
	}
	return &tagImpl{
		id:        anID,
		projectID: aProjectID,
		name:      aName,
		tagType:   aType,
		version:   aVersion,
	}, nil
}

func (t tagImpl) ID() id.ID[Tag]                    { return t.id }
func (t tagImpl) ProjectID() id.ID[project.Project] { return t.projectID }
func (t tagImpl) Name() TagName                     { return t.name }
func (t tagImpl) Type() TagType                     { return t.tagType }
func (t tagImpl) Version() version.Version[Tag]     { return t.version }

func (t *tagImpl) Delete() Tag {
	deleted := *t
	deleted.pending = append(slices.Clone(t.pending), NewTagDeletedEvent(t.id, t.projectID))
	return &deleted
}

func (t tagImpl) PendingEvents() []event.Event {
	return slices.Clone(t.pending)
}

// String returns a string representation of the tag.
func (t tagImpl) String() string {
	return fmt.Sprintf("Tag: %s, Type: %s, Version: %d", t.name, t.tagType, t.version)
}
