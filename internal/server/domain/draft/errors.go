package draft

import (
	"errors"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
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
	Scope    editlock.Scope
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
