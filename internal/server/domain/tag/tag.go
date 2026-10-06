package tag

import (
	"fmt"
	"slices"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
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

	// SetValue changes the Tag's value and Quality and records a
	// TagUpdatedEvent carrying the state Save stores.
	SetValue(any, TagQuality) error

	// Delete records a TagDeletedEvent; the repository's Delete removes the Tag.
	Delete()

	event.Recorder
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
	pending []event.Event
}

var _ Tag = (*tagImpl)(nil)

// CreateTag creates a new, not yet saved tag and records a TagCreatedEvent.
// Returns an error if the type or quality is the invalid zero value.
func CreateTag(anID id.ID[Tag], aName TagName, aType TagType, aValue TagValue, aQuality TagQuality) (Tag, error) {
	t, err := ReconstituteTag(anID, aName, aType, aValue, aQuality, version.Initial[Tag]())
	if err != nil {
		return nil, err
	}
	impl := t.(*tagImpl)
	impl.pending = []event.Event{NewTagCreatedEvent(anID)}
	return impl, nil
}

// ReconstituteTag rebuilds a tag as stored, at the given version; it records
// no event. For repositories and the decoding of stored events.
// Returns an error if the type or quality is the invalid zero value.
func ReconstituteTag(anID id.ID[Tag], aName TagName, aType TagType, aValue TagValue, aQuality TagQuality, aVersion version.Version[Tag]) (Tag, error) {
	if !aType.IsValid() {
		return nil, fmt.Errorf("cannot create tag: invalid type")
	}
	if !aQuality.IsValid() {
		return nil, fmt.Errorf("cannot create tag: invalid quality")
	}
	return &tagImpl{
		id:      anID,
		name:    aName,
		tagType: aType,
		value:   aValue,
		quality: aQuality,
		version: aVersion,
	}, nil
}

func (t tagImpl) ID() id.ID[Tag]                { return t.id }
func (t tagImpl) Name() TagName                 { return t.name }
func (t tagImpl) Type() TagType                 { return t.tagType }
func (t tagImpl) Value() TagValue               { return t.value }
func (t tagImpl) Quality() TagQuality           { return t.quality }
func (t tagImpl) Version() version.Version[Tag] { return t.version }

// SetValue updates the tag's value and quality.
// Returns an error if the quality is the invalid zero value.
func (t *tagImpl) SetValue(aValue any, aQuality TagQuality) error {
	if !aQuality.IsValid() {
		return fmt.Errorf("cannot update tag value: invalid quality")
	}
	newValue, err := t.tagType.NewTagValue(aValue)
	if err != nil {
		return fmt.Errorf("cannot update tag value: %w", err)
	}
	t.value = newValue
	t.quality = aQuality
	// Save raises the Version by one, so the event carries the state stored.
	saved := tagImpl{
		id: t.id, name: t.name, tagType: t.tagType,
		value: t.value, quality: t.quality, version: t.version.Next(),
	}
	t.pending = append(t.pending, NewTagUpdatedEvent(&saved))
	return nil
}

func (t *tagImpl) Delete() {
	t.pending = append(t.pending, NewTagDeletedEvent(t.id))
}

func (t tagImpl) PendingEvents() []event.Event {
	return slices.Clone(t.pending)
}

// String returns a string representation of the tag.
func (t tagImpl) String() string {
	return fmt.Sprintf(
		"Tag: %s, Type: %s, Value: %v, Quality: %s, Version: %d",
		t.name, t.tagType, t.value, t.quality, t.version,
	)
}
