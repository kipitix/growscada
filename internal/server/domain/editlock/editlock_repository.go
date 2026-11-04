package editlock

import (
	"context"
	"time"
)

// Repository stores the EditLocks of the Drafts in the Engineering node's
// schema.
type Repository interface {
	// Find returns the lock of the Draft scope and whether a row exists. A
	// Draft never locked has none: its lock is free at Version 0.
	Find(ctx context.Context, scope Scope) (EditLock, bool, error)

	// Create stores the lock of a Draft that has none.
	// Returns an error if one already exists (it was created concurrently).
	Create(ctx context.Context, lock EditLock) (EditLock, error)

	// Save stores a lock read at its current Version: the stored row is
	// replaced if it is still at that Version, and the lock's Version is
	// raised once. The returned lock carries the new Version and no events.
	// Returns ErrEditLockVersionStale if the stored Version does not match.
	Save(ctx context.Context, lock EditLock) (EditLock, error)

	// FindAllHeld returns every held lock, whatever its Scope.
	FindAllHeld(ctx context.Context) ([]EditLock, error)

	// Touch raises the LastSeen of the lock of scope held by key to at: it
	// changes no holder and raises no Version, so it records no event and
	// needs no CAS. An unknown or free lock is no error.
	Touch(ctx context.Context, scope Scope, key Key, at time.Time) error
}
