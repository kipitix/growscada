package tag

import (
	"fmt"
)

const (
	TagVersionInitial   = 0
	TagVersionCommitted = 1
)

// Tag - interface representing a tag.
// A tag is the fundamental system entity containing an identifier, name, type, value, and quality.
type Tag interface {
	ID() TagID
	Name() TagName
	Type() TagType
	Value() TagValue
	Quality() TagQuality
	Version() int

	SetValue(any, TagQuality) error
	IncrementVersion()

	fmt.Stringer
}

// tagImpl - tag implementation struct
type tagImpl struct {
	id      TagID
	name    TagName
	tagType TagType
	value   TagValue
	quality TagQuality
	// TODO : make VO TagVersion
	version int
}

var _ Tag = (*tagImpl)(nil)

// NewTag creates a new tag with the given identifier, name, and type.
// Sets the initial value to nil and quality to TagQualityBad.
// version is initialized to 0.
func NewTag(anID TagID, aName TagName, aType TagType, aValue TagValue, aQuality TagQuality, aVersion int) (Tag, error) {
	newTag := &tagImpl{
		id:      anID,
		name:    aName,
		tagType: aType,
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

// Type returns the tag type
func (t tagImpl) Type() TagType {
	return t.tagType
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

// SetValue updates the tag's value and quality
func (t *tagImpl) SetValue(aValue any, aQuality TagQuality) error {
	newValue, err := t.tagType.NewTagValue(aValue)
	if err != nil {
		return fmt.Errorf("cannot update tag value: %w", err)
	}
	// Set value and quality simultaneously
	t.value = newValue
	t.quality = aQuality
	return nil
}

// IncrementVersion increments the tag version by one
func (t *tagImpl) IncrementVersion() {
	t.version++
}

// String returns a string representation of the tag
func (t tagImpl) String() string {
	return fmt.Sprintf(
		"Tag: %s, Type: %s, Value: %v, Quality: %s, Version: %d",
		t.name, t.tagType, t.value, t.quality, t.version,
	)
}
