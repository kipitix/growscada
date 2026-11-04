package appdto

import (
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
)

// EditLock is the state of a Draft's EditLock as the API shows it.
type EditLock struct {
	Held    bool
	Holder  *EditLockHolder
	Version int
}

// EditLockHolder is who holds a Draft's EditLock.
type EditLockHolder struct {
	Key   uuid.UUID
	Name  string
	Since time.Time
}

// NewEditLock is the state of an EditLock.
func NewEditLock(state editlock.Snapshot) EditLock {
	lock := EditLock{Held: state.Held, Version: state.Version}
	if state.Held {
		lock.Holder = &EditLockHolder{
			Key:   state.Holder.Key().UUID(),
			Name:  state.Holder.Name(),
			Since: state.Since,
		}
	}
	return lock
}
