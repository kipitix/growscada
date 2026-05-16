package id

import (
	"fmt"

	"github.com/google/uuid"
)

// ID - UUID-based aggregates identifier.
// Represents a value object for unique aggregates identification.
// Generic type T is used to specify the type of the aggregate.
type ID[T any] struct {
	uuid uuid.UUID
}

// Interfaces for AggregateID.
var _ fmt.Stringer = ID[int]{}

// IDOption - option function for configuring ID creation.
type IDOption[T any] func(*ID[T]) error

// NewID generates a new random ID.
// Returns an error if any option fails.
func NewID[T any](opts ...IDOption[T]) (ID[T], error) {
	newID := ID[T]{}

	for _, opt := range opts {
		if err := opt(&newID); err != nil {
			return ID[T]{}, err
		}
	}

	if newID.uuid == uuid.Nil {
		newID.uuid = uuid.New()
	}

	return newID, nil
}

// IDWithUUID allows specifying an existing UUID for ID creation.
func IDWithUUID[T any](anUUID uuid.UUID) IDOption[T] {
	return func(id *ID[T]) error {
		id.uuid = anUUID
		return nil
	}
}

// IDWithString parses a UUID string and returns an option for AggregateID creation.
// Returns an error if the string is not a valid UUID.
func IDWithString[T any](s string) (IDOption[T], error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return nil, err
	}
	return IDWithUUID[T](u), nil
}

// UUID converts ID to a uuid.UUID.
func (id ID[T]) UUID() uuid.UUID {
	return id.uuid
}

// String implements [fmt.Stringer].
func (id ID[T]) String() string {
	return id.uuid.String()
}
