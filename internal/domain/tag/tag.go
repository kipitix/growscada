package tag

import (
	"fmt"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/version"
)

// Tag - interface representing a tag.
// A tag is the fundamental system entity containing an identifier, name, type, value, and quality.
// Aggregate
type Tag interface {
	ID() id.ID[Tag]
	Name() TagName
	Type() TagType
	Value() TagValue
	Quality() TagQuality
	Version() version.Version[Tag]

	SetValue(any, TagQuality) error

	fmt.Stringer
}

// tagImpl - tag implementation struct
type tagImpl struct {
	id      id.ID[Tag]
	name    TagName
	tagType TagType
	value   TagValue
	quality TagQuality
	version version.Version[Tag]
}

var _ Tag = (*tagImpl)(nil)

// NewTag creates a new tag with the given identifier, name, type, value, quality and version.
func NewTag(anID id.ID[Tag], aName TagName, aType TagType, aValue TagValue, aQuality TagQuality, aVersion version.Version[Tag]) (Tag, error) {
	return &tagImpl{
		id:      anID,
		name:    aName,
		tagType: aType,
		value:   aValue,
		quality: aQuality,
		version: aVersion,
	}, nil
}

func (t tagImpl) ID() id.ID[Tag]            { return t.id }
func (t tagImpl) Name() TagName              { return t.name }
func (t tagImpl) Type() TagType              { return t.tagType }
func (t tagImpl) Value() TagValue            { return t.value }
func (t tagImpl) Quality() TagQuality        { return t.quality }
func (t tagImpl) Version() version.Version[Tag] { return t.version }

// SetValue updates the tag's value and quality.
func (t *tagImpl) SetValue(aValue any, aQuality TagQuality) error {
	newValue, err := t.tagType.NewTagValue(aValue)
	if err != nil {
		return fmt.Errorf("cannot update tag value: %w", err)
	}
	t.value = newValue
	t.quality = aQuality
	return nil
}

// String returns a string representation of the tag.
func (t tagImpl) String() string {
	return fmt.Sprintf(
		"Tag: %s, Type: %s, Value: %v, Quality: %s, Version: %d",
		t.name, t.tagType, t.value, t.quality, t.version,
	)
}
