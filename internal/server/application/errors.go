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

// ErrRaceLost matches (errors.Is) the error of an operation that states no
// expected Version and lost the write race every time it was retried
// (retryOnRace). The error also matches the aggregate's conflict; unlike an
// edit conflict there is nothing for the caller to reload, only to retry later.
var ErrRaceLost = errors.New("write race lost on every attempt")

// ErrNothingToDeploy matches (errors.Is) a Deploy refused because the Draft is
// the Revision the Project runs unchanged (task 35, Q9), as
// library.ErrDraftChanged refuses an unchanged Release.
var ErrNothingToDeploy = errors.New("nothing to deploy")

// ErrRevisionCopyNotSaved matches (errors.Is) a Deploy whose Revision the
// Runtime node created but whose copy the Engineering node failed to save
// (task 35, Q8): the Revision is active and the Project runs it; only the
// Engineering node's copy — a cache for the UI — is missing, and the next
// Deploy compares with the Revision, not the copy.
var ErrRevisionCopyNotSaved = errors.New("revision is active, its copy is not saved")

// ErrNoRevisionToDiscard matches (errors.Is) a Discard refused because the
// Project has no Revision to reset its Draft to (task 35, Q10).
var ErrNoRevisionToDiscard = errors.New("nothing to discard: the project runs no revision")
