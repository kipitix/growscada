package tag

import (
	"fmt"
)

const (
	TagVersionInitial = 0
)

// Tag - interface representing a tag.
// A tag is the fundamental system entity containing an identifier, name, type, value, and quality.
type Tag interface {
	ID() TagID
	Name() TagName
	Kind() TagKind
	Value() TagValue
	Quality() TagQuality
	Version() int

	UpdateValue(any, TagQuality) error
	IncrementVersion()
}

// tagImpl - tag implementation struct
type tagImpl struct {
	id      TagID
	name    TagName
	kind    TagKind
	value   TagValue
	quality TagQuality
	// TODO : make VO TagVersion
	version int
}

var _ Tag = (*tagImpl)(nil)

// NewTag creates a new tag with the given identifier, name, and type.
// Sets the initial value to nil and quality to TagQualityBad.
// version is initialized to 0.
func NewTag(anID TagID, aName TagName, aKind TagKind, aValue TagValue, aQuality TagQuality, aVersion int) (Tag, error) {
	newTag := &tagImpl{
		id:      anID,
		name:    aName,
		kind:    aKind,
		value:   aValue,
		quality: aQuality,
		version: aVersion,
	}

	return newTag, nil
}

// ID returns the tag identifier
func (t tagImpl) ID() TagID {
	return t.id
}

// Name returns the tag name
func (t tagImpl) Name() TagName {
	return t.name
}

// Kind returns the tag type
func (t tagImpl) Kind() TagKind {
	return t.kind
}

func (t tagImpl) Value() TagValue {
	return t.value
}

// Quality returns the tag quality
func (t tagImpl) Quality() TagQuality {
	return t.quality
}

// Version returns the current tag version
func (t tagImpl) Version() int {
	return t.version
}

// UpdateValue updates the tag's value and quality
func (t *tagImpl) UpdateValue(aValue any, aQuality TagQuality) error {
	t.quality = aQuality
	newValue, err := t.kind.NewTagValue(aValue)
	if err != nil {
		return fmt.Errorf("cannot update tag value: %w", err)
	}
	t.value = newValue
	return nil
}

// IncrementVersion increments the tag version by one
func (t *tagImpl) IncrementVersion() {
	t.version++
}
