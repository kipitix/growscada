package tag

import (
	"fmt"

	"github.com/google/uuid"
)

// TagID - UUID-based tag identifier.
// Represents a value object for unique tag identification.
type TagID uuid.UUID

// Interfaces for TagID.
var _ fmt.Stringer = TagID{}

// NewTagID generates a new random TagID.
// Uses uuid.New() to create a unique identifier.
func NewTagID(opts ...TagIDOption) TagID {
	// Create a new UUID
	tagID := TagID(uuid.New())

	// Apply options
	for _, opt := range opts {
		opt(&tagID)
	}

	return tagID
}

// TagIDOption - option function for configuring TagID creation.
type TagIDOption func(*TagID)

// TagIDWithUUID allows specifying an existing UUID for TagID creation.
// Used when a TagID needs to be created from an existing UUID.
func TagIDWithUUID(uuid uuid.UUID) TagIDOption {
	return func(tagID *TagID) {
		*tagID = TagID(uuid)
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
	id, err := uuid.Parse(s)
	if err != nil {
		return TagID{}, err
	}
	return TagID(id), nil
}

// UUID converts TagID to a uuid.UUID.
func (id TagID) UUID() uuid.UUID {
	return uuid.UUID(id)
}

// String implements [fmt.Stringer].
func (id TagID) String() string {
	return uuid.UUID(id).String()
}
