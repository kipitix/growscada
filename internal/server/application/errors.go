package application

import (
	"errors"
	"fmt"
)

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
// library.ErrNothingToRelease refuses an unchanged Release.
var ErrNothingToDeploy = errors.New("nothing to deploy")

// ErrNoRevisionToDiscard matches (errors.Is) a Discard refused because the
// Project has no Revision to reset its Draft to (task 35, Q10).
var ErrNoRevisionToDiscard = errors.New("nothing to discard: the project runs no revision")

// ErrInvalidProjectFile matches (errors.Is) a project file that does not
// validate: its problems are the FileIssues of a ProjectFileError (task 39,
// ADR 0018).
var ErrInvalidProjectFile = errors.New("invalid project file")

// ErrNothingToOpen matches (errors.Is) an open of a project file whose
// canonical form is the Draft the Project has already (ADR 0018), as
// ErrNothingToDeploy refuses an unchanged Draft.
var ErrNothingToOpen = errors.New("nothing to open: the draft is the file")

// ErrDraftNotDeployable matches (errors.Is) an export of a Draft the
// ProjectFileBuilder refuses: an inconsistent Draft is not a file (ADR 0018).
var ErrDraftNotDeployable = errors.New("the draft is not deployable")

// ErrRevisionNotDiscardable matches (errors.Is) a Discard refused because the
// Revision names Library Releases this server does not have (ADR 0018): the
// pins cannot be written, and the Draft is left as it is.
var ErrRevisionNotDiscardable = errors.New("the revision cannot be discarded here: library releases are missing")

// DraftNotDeployableError is an export of a Draft the builder refused, with
// the builder's reason.
type DraftNotDeployableError struct {
	Reason string
}

func (e DraftNotDeployableError) Error() string { return "the draft is not deployable: " + e.Reason }

func (e DraftNotDeployableError) Is(target error) bool { return target == ErrDraftNotDeployable }

// RevisionNotDiscardableError is a Discard refused because the Revision's
// pins are not local, naming them.
type RevisionNotDiscardableError struct {
	Issues []FileIssue
}

func (e RevisionNotDiscardableError) Error() string {
	return "the revision cannot be discarded here: " + fmt.Sprintf("%d library releases are missing", len(e.Issues))
}

func (e RevisionNotDiscardableError) Is(target error) bool {
	return target == ErrRevisionNotDiscardable
}
