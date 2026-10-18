package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// TagTypes - the TagTypes of Tags by their IDs, as the caller of a Scene
// change read them: the Scene never reads Tags itself (ADR 0010). A Tag
// missing from TagTypes does not exist.
// Value Object.
type TagTypes struct {
	types map[id.ID[tag.Tag]]tag.TagType
}

// NewTagTypes returns TagTypes holding the given types. It takes someTypes
// over without copying: the caller passes a map of its own that nothing else
// changes.
func NewTagTypes(someTypes map[id.ID[tag.Tag]]tag.TagType) TagTypes {
	return TagTypes{types: someTypes}
}

// Of returns the TagType of the Tag with the given ID; false if TagTypes has
// no such Tag.
func (t TagTypes) Of(tagID id.ID[tag.Tag]) (tag.TagType, bool) {
	tagType, ok := t.types[tagID]
	return tagType, ok
}

// BoundTagIDs returns the IDs of the Tags someWidgets bind, each once, in
// the order they are first bound.
func BoundTagIDs(someWidgets ...Widget) []id.ID[tag.Tag] {
	var out []id.ID[tag.Tag]
	seen := make(map[id.ID[tag.Tag]]struct{})
	for _, w := range someWidgets {
		for _, b := range w.PortBindings() {
			if _, ok := seen[b.TagID()]; !ok {
				seen[b.TagID()] = struct{}{}
				out = append(out, b.TagID())
			}
		}
	}
	return out
}
