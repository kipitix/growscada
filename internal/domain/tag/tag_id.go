package tag

import (
	"fmt"

	"github.com/google/uuid"
)

// TagID - UUID-based tag identifier.
// Represents a value object for unique tag identification.
type TagID struct {
	uuid uuid.UUID
}

// Interfaces for TagID.
var _ fmt.Stringer = TagID{}

// NewTagID generates a new random TagID.
// Uses uuid.New() to create a unique identifier.
func NewTagID(opts ...TagIDOption) TagID {
	tagID := TagID{}

	// Apply options
	for _, opt := range opts {
		opt(&tagID)
	}

	// Create a new UUID if none provided
	if tagID.uuid == uuid.Nil {
		tagID.uuid = uuid.New()
	}

	return tagID
}

// TagIDOption - option function for configuring TagID creation.
type TagIDOption func(*TagID)

// TagIDWithUUID allows specifying an existing UUID for TagID creation.
// Used when a TagID needs to be created from an existing UUID.
func TagIDWithUUID(anUUID uuid.UUID) TagIDOption {
	return func(tagID *TagID) {
		tagID.uuid = anUUID
	}
}

// MustParseTagID creates a TagID from a UUID string.
// Panics if the string is not a valid UUID.
func MustParseTagID(s string) TagID {
	id, err := ParseTagID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// ParseTagID creates a TagID from a UUID string.
// Returns an error if the string is not a valid UUID.
func ParseTagID(s string) (TagID, error) {
	uuid, err := uuid.Parse(s)
	if err != nil {
		return TagID{}, err
	}
	return NewTagID(TagIDWithUUID(uuid)), nil
}

// UUID converts TagID to a uuid.UUID.
func (id TagID) UUID() uuid.UUID {
	return id.uuid
}

// String implements [fmt.Stringer].
func (id TagID) String() string {
	return id.uuid.String()
}
