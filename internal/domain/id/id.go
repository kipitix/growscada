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
type IDOption[T any] func(*ID[T])

// NewID generates a new random ID.
// Returns an error if any option fails.
func NewID[T any](opts ...IDOption[T]) ID[T] {
	newID := ID[T]{}

	for _, opt := range opts {
		opt(&newID)
	}

	if newID.uuid == uuid.Nil {
		newID.uuid = uuid.New()
	}

	return newID
}

// IDWithUUID allows specifying an existing UUID for ID creation.
func IDWithUUID[T any](anUUID uuid.UUID) IDOption[T] {
	return func(id *ID[T]) {
		id.uuid = anUUID
	}
}

// UUID converts ID to a uuid.UUID.
func (id ID[T]) UUID() uuid.UUID {
	return id.uuid
}

// String implements [fmt.Stringer].
func (id ID[T]) String() string {
	return id.uuid.String()
}
