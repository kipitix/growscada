// Package tagstate is the Runtime node's model of Tags (ADR 0011): a Tag's
// state — its current value and Quality — with the copy of the Tag's
// definition a write is checked against. The Tag definition itself is the
// Engineering node's (package tag).
package tagstate

import (
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Seq numbers the writes of Tag states: each write takes a number higher
// than every write before it, of one sequence of the Runtime node, so a
// reader tells the newer of two states of one Tag. A state no supplier has
// written yet has none (0).
// Value Object.
type Seq int64

// Definition is the Runtime node's copy of a Tag definition: the Tag's name,
// TagType and the definition's Version, of the Tag's Project.
// Value Object.
type Definition struct {
	TagID     id.ID[tag.Tag]
	ProjectID id.ID[project.Project]
	Name      tag.TagName
	Type      tag.TagType
	Version   version.Version[tag.Tag]
}

// NewDefinition returns the copy of the definition aTag.
func NewDefinition(aTag tag.Tag) Definition {
	return Definition{TagID: aTag.ID(), ProjectID: aTag.ProjectID(), Name: aTag.Name(), Type: aTag.Type(), Version: aTag.Version()}
}

// TagState is a Tag's current value and Quality, held by the Runtime node,
// never part of the Draft or a Revision. It has a single writer — the Tag's
// supplier, or people — so a write states no expected Version: the latest
// write wins, numbered by its Seq.
// Immutable aggregate: Write returns a new TagState.
type TagState struct {
	definition Definition
	value      tag.TagValue
	quality    tag.TagQuality
	seq        Seq
}

// Initial returns the state of a Tag no supplier has written yet: its
// TagType's zero value with Quality Bad.
// Returns an error if the definition's TagType is invalid.
func Initial(aDefinition Definition) (TagState, error) {
	zero, err := aDefinition.Type.ZeroValue()
	if err != nil {
		return TagState{}, fmt.Errorf("cannot create tag state: %w", err)
	}
	return TagState{definition: aDefinition, value: zero, quality: tag.TagQualityBad}, nil
}

// Reconstitute rebuilds a state as stored. For repositories.
// Returns an error if the quality is the invalid zero value or the value is
// missing.
func Reconstitute(aDefinition Definition, aValue tag.TagValue, aQuality tag.TagQuality, aSeq Seq) (TagState, error) {
	if aValue == nil {
		return TagState{}, fmt.Errorf("cannot create tag state: no value")
	}
	if !aQuality.IsValid() {
		return TagState{}, fmt.Errorf("cannot create tag state: invalid quality")
	}
	return TagState{definition: aDefinition, value: aValue, quality: aQuality, seq: aSeq}, nil
}

func (s TagState) Definition() Definition            { return s.definition }
func (s TagState) TagID() id.ID[tag.Tag]             { return s.definition.TagID }
func (s TagState) ProjectID() id.ID[project.Project] { return s.definition.ProjectID }
func (s TagState) Value() tag.TagValue               { return s.value }
func (s TagState) Quality() tag.TagQuality           { return s.quality }
func (s TagState) Seq() Seq                          { return s.seq }

// Write returns the state with the new value and Quality, the value parsed
// for the Tag's TagType. Its Seq is the one of the state written over: the
// repository's Write numbers it.
// Returns an error if the quality is the invalid zero value or the value
// does not fit the TagType.
func (s TagState) Write(aValue any, aQuality tag.TagQuality) (TagState, error) {
	if !aQuality.IsValid() {
		return TagState{}, fmt.Errorf("cannot update tag value: invalid quality")
	}
	newValue, err := s.definition.Type.NewTagValue(aValue)
	if err != nil {
		return TagState{}, fmt.Errorf("cannot update tag value: %w", err)
	}
	s.value = newValue
	s.quality = aQuality
	return s, nil
}

// String returns a string representation of the state.
func (s TagState) String() string {
	return fmt.Sprintf("TagState: %s, Value: %v, Quality: %s, Seq: %d", s.definition.Name, s.value, s.quality, s.seq)
}
