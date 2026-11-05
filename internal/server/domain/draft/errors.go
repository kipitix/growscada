package draft

import "errors"

// ErrDraftConflict is the one edit conflict of a Draft's content: the Version
// the write stated is not the Draft's current one — another write of the same
// Draft happened since the client read it (ADR 0016). Every write of the
// Draft's content takes the same Version, so one sentinel serves the Tag, the
// Scene, the Device, the Environment, the pair and the Library Pin.
var ErrDraftConflict = errors.New("draft version conflict")

// ErrDraftNotFound is returned by a read of a Draft whose Project or Library
// does not exist.
var ErrDraftNotFound = errors.New("draft not found")
