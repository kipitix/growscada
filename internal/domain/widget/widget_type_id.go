package widget

import (
	"fmt"

	"github.com/google/uuid"
)

// WidgetTypeID - UUID-based widget type identifier.
// Value Object.
type WidgetTypeID struct {
	uuid uuid.UUID
}

var _ fmt.Stringer = WidgetTypeID{}

// NewWidgetTypeID generates a new random WidgetTypeID.
func NewWidgetTypeID(opts ...WidgetTypeIDOption) WidgetTypeID {
	id := WidgetTypeID{}

	for _, opt := range opts {
		opt(&id)
	}

	if id.uuid == uuid.Nil {
		id.uuid = uuid.New()
	}

	return id
}

// WidgetTypeIDOption - option function for configuring WidgetTypeID creation.
type WidgetTypeIDOption func(*WidgetTypeID)

// WidgetTypeIDWithUUID allows specifying an existing UUID for WidgetTypeID creation.
func WidgetTypeIDWithUUID(anUUID uuid.UUID) WidgetTypeIDOption {
	return func(id *WidgetTypeID) {
		id.uuid = anUUID
	}
}

// MustParseWidgetTypeID creates a WidgetTypeID from a UUID string.
// Panics if the string is not a valid UUID.
func MustParseWidgetTypeID(s string) WidgetTypeID {
	id, err := ParseWidgetTypeID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// ParseWidgetTypeID creates a WidgetTypeID from a UUID string.
// Returns an error if the string is not a valid UUID.
func ParseWidgetTypeID(s string) (WidgetTypeID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return WidgetTypeID{}, err
	}
	return NewWidgetTypeID(WidgetTypeIDWithUUID(u)), nil
}

// UUID converts WidgetTypeID to a uuid.UUID.
func (id WidgetTypeID) UUID() uuid.UUID {
	return id.uuid
}

// String implements [fmt.Stringer].
func (id WidgetTypeID) String() string {
	return id.uuid.String()
}
