package scene

import (
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// TagTypes - the TagTypes of Tags of one Project by their IDs, as the caller
// of a Scene change read them: the Scene never reads Tags itself (ADR 0010).
// A Tag missing from TagTypes does not exist in that Project.
// Value Object.
type TagTypes struct {
	projectID id.ID[project.Project]
	types     map[id.ID[tag.Tag]]tag.TagType
}

// NewTagTypes returns TagTypes holding the given types of Tags of the Project
// aProjectID. It takes someTypes over without copying: the caller passes a
// map of its own that nothing else changes.
func NewTagTypes(aProjectID id.ID[project.Project], someTypes map[id.ID[tag.Tag]]tag.TagType) TagTypes {
	return TagTypes{projectID: aProjectID, types: someTypes}
}

// Of returns the TagType of the Tag with the given ID if it is a Tag of the
// Project aProjectID; false if TagTypes has no such Tag of that Project.
func (t TagTypes) Of(aProjectID id.ID[project.Project], tagID id.ID[tag.Tag]) (tag.TagType, bool) {
	if aProjectID != t.projectID {
		return tag.TagType{}, false
	}
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
