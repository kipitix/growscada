package indicator_type

import (
	"fmt"

	"github.com/google/uuid"
)

// IndicatorTypeID - UUID-based indicator type identifier.
type IndicatorTypeID uuid.UUID

// Interfaces for IndicatorTypeID.
var _ fmt.Stringer = IndicatorTypeID{}

// NewIndicatorTypeID generates a new random IndicatorTypeID.
func NewIndicatorTypeID(opts ...IndicatorTypeIDOption) IndicatorTypeID {
	id := IndicatorTypeID(uuid.New())

	for _, opt := range opts {
		opt(&id)
	}

	return id
}

// IndicatorTypeIDOption - option function for configuring IndicatorTypeID creation.
type IndicatorTypeIDOption func(*IndicatorTypeID)

// IndicatorTypeIDWithUUID allows specifying an existing UUID for IndicatorTypeID creation.
func IndicatorTypeIDWithUUID(u uuid.UUID) IndicatorTypeIDOption {
	return func(id *IndicatorTypeID) {
		*id = IndicatorTypeID(u)
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
func ParseIndicatorTypeID(s string) (IndicatorTypeID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return IndicatorTypeID{}, err
	}
	return IndicatorTypeID(id), nil
}

// UUID converts IndicatorTypeID to a uuid.UUID.
func (id IndicatorTypeID) UUID() uuid.UUID {
	return uuid.UUID(id)
}

// String implements [fmt.Stringer].
func (id IndicatorTypeID) String() string {
	return uuid.UUID(id).String()
}
