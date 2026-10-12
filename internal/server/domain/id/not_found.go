package id

import (
	"fmt"

	"github.com/google/uuid"
)

// NotFoundError says which entity was not found: Err is the sentinel of its
// kind (errors.Is matches it), ID is the entity's ID.
type NotFoundError struct {
	Err error
	ID  uuid.UUID
}

// NotFound marks sentinel as about the entity anID.
func NotFound[T any](sentinel error, anID ID[T]) error {
	return NotFoundError{Err: sentinel, ID: anID.UUID()}
}

func (e NotFoundError) Error() string { return fmt.Sprintf("%v: %s", e.Err, e.ID) }

func (e NotFoundError) Unwrap() error { return e.Err }
