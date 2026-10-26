package scene

import (
	"errors"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// ErrTagTypesOfAnotherProject is returned by a Scene change handed the
// TagTypes of another Project than the Scene's: a programming error, which
// must not pass for Tags that do not exist (and lose bindings to them).
var ErrTagTypesOfAnotherProject = errors.New("tag types of another project")

// TagTypes - the TagTypes of Tags of one Project by their IDs, as the caller
// of a Scene change read them: the Scene never reads Tags itself (ADR 0010).
// A Tag missing from TagTypes does not exist in that Project; a Scene takes
// only the TagTypes of its own Project.
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

// Of returns the TagType of the Tag with the given ID; false if TagTypes has
// no such Tag of its Project.
func (t TagTypes) Of(tagID id.ID[tag.Tag]) (tag.TagType, bool) {
	tagType, ok := t.types[tagID]
	return tagType, ok
}

// checkFor fails unless t holds Tags of the Project aProjectID.
func (t TagTypes) checkFor(aProjectID id.ID[project.Project]) error {
	if t.projectID != aProjectID {
		return fmt.Errorf("%w: tag types of %s, scene of %s", ErrTagTypesOfAnotherProject, t.projectID, aProjectID)
	}
	return nil
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
