package indicator_type

import (
	"fmt"

	"github.com/google/uuid"
)

// IndicatorTypeID - UUID-based indicator type identifier.
// Represents a value object for unique indicator type identification.
type IndicatorTypeID struct {
	uuid uuid.UUID
}

// Interfaces for IndicatorTypeID.
var _ fmt.Stringer = IndicatorTypeID{}

// NewIndicatorTypeID generates a new random IndicatorTypeID.
// Uses uuid.New() to create a unique identifier.
func NewIndicatorTypeID(opts ...IndicatorTypeIDOption) IndicatorTypeID {
	id := IndicatorTypeID{}

	// Apply options
	for _, opt := range opts {
		opt(&id)
	}

	// Create a new UUID if none provided
	if id.uuid == uuid.Nil {
		id.uuid = uuid.New()
	}

	return id
}

// IndicatorTypeIDOption - option function for configuring IndicatorTypeID creation.
type IndicatorTypeIDOption func(*IndicatorTypeID)

// IndicatorTypeIDWithUUID allows specifying an existing UUID for IndicatorTypeID creation.
// Used when an IndicatorTypeID needs to be created from an existing UUID.
func IndicatorTypeIDWithUUID(anUUID uuid.UUID) IndicatorTypeIDOption {
	return func(id *IndicatorTypeID) {
		id.uuid = anUUID
	}
}

// MustParseIndicatorTypeID creates an IndicatorTypeID from a UUID string.
// Panics if the string is not a valid UUID.
func MustParseIndicatorTypeID(s string) IndicatorTypeID {
	id, err := ParseIndicatorTypeID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// ParseIndicatorTypeID creates an IndicatorTypeID from a UUID string.
// Returns an error if the string is not a valid UUID.
func ParseIndicatorTypeID(s string) (IndicatorTypeID, error) {
	uuid, err := uuid.Parse(s)
	if err != nil {
		return IndicatorTypeID{}, err
	}
	return NewIndicatorTypeID(IndicatorTypeIDWithUUID(uuid)), nil
}

// UUID converts IndicatorTypeID to a uuid.UUID.
func (id IndicatorTypeID) UUID() uuid.UUID {
	return id.uuid
}

// String implements [fmt.Stringer].
func (id IndicatorTypeID) String() string {
	return id.uuid.String()
}
