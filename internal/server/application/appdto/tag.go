package appdto

import (
	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// Tag is the application-layer DTO for tag data: its definition, from the
// Draft, with its state, from the Runtime node (ADR 0011). Fields have no
// JSON tags — serialization is handled at the interface layer.
type Tag struct {
	ID   uuid.UUID
	Name string
	Type string
	// Version is the Tag definition's.
	Version int
	Value   string
	Quality string
	// Seq numbers the write of the state; 0 until a supplier writes one.
	Seq int64
}

// CreateTagInput holds the input data for creating a tag: its definition.
// Its state starts as its TagType's zero value with Quality bad.
type CreateTagInput struct {
	Name string
	Type string
}

// TagDefinition is a Tag definition as the Runtime node is given it
// (RuntimeNode).
type TagDefinition struct {
	ID      uuid.UUID
	Name    string
	Type    string
	Version int
}

// TagState is a Tag's state as the Runtime node holds it (RuntimeNode).
type TagState struct {
	ID      uuid.UUID
	Value   string
	Quality string
	Seq     int64
}

// NewTagDefinition returns the definition of aTag for the Runtime node.
func NewTagDefinition(aTag tag.Tag) TagDefinition {
	return TagDefinition{
		ID:      aTag.ID().UUID(),
		Name:    aTag.Name().String(),
		Type:    aTag.Type().String(),
		Version: aTag.Version().Number(),
	}
}

// InitialTagState returns the state of aTag before a supplier writes one:
// its TagType's zero value with Quality bad.
func InitialTagState(aTag tag.Tag) TagState {
	state := TagState{ID: aTag.ID().UUID(), Quality: tag.TagQualityBad.String()}
	if zero, err := aTag.Type().ZeroValue(); err == nil {
		state.Value = zero.String()
	}
	return state
}

// NewTag creates a Tag DTO from the tag.Tag domain aggregate and its state.
func NewTag(aTag tag.Tag, aState TagState) Tag {
	return Tag{
		ID:      aTag.ID().UUID(),
		Name:    aTag.Name().String(),
		Type:    aTag.Type().String(),
		Version: aTag.Version().Number(),
		Value:   aState.Value,
		Quality: aState.Quality,
		Seq:     aState.Seq,
	}
}
