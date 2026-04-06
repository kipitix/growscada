package tag

import "fmt"

type Tag interface {
	ID() TagID
	Name() string

	UpdateValue(TagValue, TagQuality) error

	IsTypeBoolean() bool
	IsTypeInteger() bool

	IsQualityBad() bool
	IsQualityUncertain() bool
	IsQualityGood() bool
	IsQualitySimulated() bool

	ValueAsBoolean() (bool, error)
	ValueAsInteger() (int, error)

	Version() int
	IncrementVersion()
}

type tagImpl struct {
	id      TagID
	name    string
	theType TagType
	value   TagValue
	quality TagQuality
	version int
}

var _ Tag = (*tagImpl)(nil)

func NewTag(anID TagID, aName string, aType TagType) Tag {
	return &tagImpl{
		id:      anID,
		name:    aName,
		theType: aType,
		value:   nil,
		quality: TagQualityBad,
		version: 0,
	}
}

func (t tagImpl) ID() TagID {
	return t.id
}

func (t tagImpl) Name() string {
	return t.name
}

func (t *tagImpl) UpdateValue(aValue TagValue, aQuality TagQuality) error {
	if !t.theType.IsValidValue(aValue) {
		return fmt.Errorf("invalid value type: expected %s, got %T", t.theType, aValue)
	}
	if aQuality == TagQualityUnknown {
		return fmt.Errorf("invalid quality: %s", aQuality)
	}
	t.value = aValue
	t.quality = aQuality
	return nil
}

func (t tagImpl) IsTypeBoolean() bool {
	return t.theType == TagTypeBoolean
}

func (t tagImpl) IsTypeInteger() bool {
	return t.theType == TagTypeInteger
}

func (t tagImpl) IsQualityBad() bool {
	return t.quality == TagQualityBad
}

func (t tagImpl) IsQualityUncertain() bool {
	return t.quality == TagQualityUncertain
}

func (t tagImpl) IsQualityGood() bool {
	return t.quality == TagQualityGood || t.quality == TagQualitySimulated
}

func (t tagImpl) IsQualitySimulated() bool {
	return t.quality == TagQualitySimulated
}

func (t tagImpl) ValueAsBoolean() (bool, error) {
	if !t.IsTypeBoolean() {
		return false, fmt.Errorf("tag is not of type boolean")
	}
	if t.value == nil {
		return false, fmt.Errorf("tag value is nil")
	}
	boolValue, ok := t.value.(bool)
	if !ok {
		return false, fmt.Errorf("tag value is not a boolean")
	}
	return boolValue, nil
}

func (t tagImpl) ValueAsInteger() (int, error) {
	if !t.IsTypeInteger() {
		return 0, fmt.Errorf("tag is not of type integer")
	}
	if t.value == nil {
		return 0, fmt.Errorf("tag value is nil")
	}
	intValue, ok := t.value.(int)
	if !ok {
		return 0, fmt.Errorf("tag value is not an integer")
	}
	return intValue, nil
}

func (t tagImpl) Version() int {
	return t.version
}

func (t *tagImpl) IncrementVersion() {
	t.version++
}
