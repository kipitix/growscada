package application

import "errors"

// ErrInvalidInput matches (errors.Is) an error caused by the caller's input:
// a value of an input DTO the domain rejects, or a reference in it to an
// entity that does not exist. Services mark only what came from the caller —
// the same domain constructors also rebuild aggregates read from a repository,
// and a stored value they reject is a server fault, not invalid input.
var ErrInvalidInput = errors.New("invalid input")

// invalidInputError marks err as invalid input without changing its message.
type invalidInputError struct {
	err error
}

func (e invalidInputError) Error() string { return e.err.Error() }

func (e invalidInputError) Unwrap() error { return e.err }

func (e invalidInputError) Is(target error) bool { return target == ErrInvalidInput }

// invalidInput marks err as caused by the caller's input (see ErrInvalidInput).
func invalidInput(err error) error {
	return invalidInputError{err: err}
}
