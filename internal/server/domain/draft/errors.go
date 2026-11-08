package draft

import (
	"errors"
	"fmt"
)

// ErrDraftConflict is the one edit conflict of a Draft's content: the Version
// the write stated is not the Draft's current one — another write of the same
// Draft happened since the client read it (ADR 0016). Every write of the
// Draft's content takes the same Version, so one sentinel serves the Tag, the
// Scene, the Device, the Environment, the pair and the Library Pin.
var ErrDraftConflict = errors.New("draft version conflict")

// ConflictError is a write that lost ErrDraftConflict, with the Version it
// stated, the one stored and the Draft it belongs to. The API names the
// current Version in its Problem, so a client can retry without rereading the
// Draft.
type ConflictError struct {
	Expected int
	Current  int
	Scope    Scope
}

func (e ConflictError) Error() string {
	return fmt.Sprintf("%v: expected %d, current %d", ErrDraftConflict, e.Expected, e.Current)
}

// Is matches ErrDraftConflict, so errors.Is keeps working for the callers that
// ask whether the write lost.
func (e ConflictError) Is(target error) bool { return target == ErrDraftConflict }

// ErrDraftNotFound is returned by a read of a Draft whose Project or Library
// does not exist.
var ErrDraftNotFound = errors.New("draft not found")

// ErrNothingToUndo is an undo with no DraftChange at the stack's position: the
// stack is empty, or everything on the other side of it was undone already
// (ADR 0017).
var ErrNothingToUndo = errors.New("nothing to undo")

// ErrNothingToRedo is a redo with no DraftChange after the stack's position:
// nothing was undone, or a new change replaced the redo tail (ADR 0017).
var ErrNothingToRedo = errors.New("nothing to redo")

// ErrNothingToSave is a Save refused because the Draft is at the Version its
// last Commit fixed: there is nothing new to save (task 72_2; ADR 0021).
var ErrNothingToSave = errors.New("nothing to save: the draft has not changed since its last commit")

// ErrCommitNotFound is a read or a rename of a Commit that the Draft does not
// have.
var ErrCommitNotFound = errors.New("commit not found")

// NothingToStepError is an undo or a redo the stack has nothing to apply to:
// it matches ErrNothingToUndo when Undo is set and ErrNothingToRedo otherwise.
// It names the Draft whose stack is empty, so the API can title the conflict
// after its Project or Library.
type NothingToStepError struct {
	Scope Scope
	Undo  bool
}

func (e NothingToStepError) Error() string {
	if e.Undo {
		return fmt.Sprintf("%v: the stack of %s has no change to undo", ErrNothingToUndo, e.Scope)
	}
	return fmt.Sprintf("%v: the stack of %s has no change to redo", ErrNothingToRedo, e.Scope)
}

// Is matches the sentinel of the command, so errors.Is keeps working for the
// callers that ask which command found the stack empty.
func (e NothingToStepError) Is(target error) bool {
	if e.Undo {
		return target == ErrNothingToUndo
	}
	return target == ErrNothingToRedo
}
