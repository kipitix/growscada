package draft

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// Repository stores the Drafts and their DraftChanges in the Engineering
// node's schema (ADR 0016, ADR 0017). One write of a Draft's content is one
// transaction over its row: the repository locks it, checks the EditLock and
// the Version the write states, runs the operation and writes the touched
// rows, the DraftChange, the new Version and stack position and the outbox's
// events.
//
// It is also the store of the EditLocks: the lock is a field of the Draft's
// row (ADR 0016), so the lock methods of editlock.Repository are served here
// instead of by a store of their own.
type Repository interface {
	editlock.Repository

	// Read returns the Draft of scope and whether a row exists; a Draft never
	// written has none. A Project or Library that does not exist has none
	// either.
	Read(ctx context.Context, scope editlock.Scope) (Draft, bool, error)

	// Update runs one write of the Draft's content: it locks the Draft's row,
	// checks the EditLock of the context and expected — a stale Version is
	// ErrDraftConflict — runs change, writes the rows its snapshots name, its
	// events and one DraftChange described by description, raises the
	// Version, appends the change at the stack and commits.
	// Returns the Draft as stored.
	Update(ctx context.Context, scope editlock.Scope, expected version.Version[Draft], description string, change func(*Session) error) (Draft, error)

	// Replace runs a write that replaces the whole content of the Draft
	// (Discard, ADR 0016): it writes the rows change touched, clears the
	// DraftChanges and the stack and commits, with no DraftChange of its own.
	// Returns the Draft as stored.
	Replace(ctx context.Context, scope editlock.Scope, change func(*Session) error) (Draft, error)

	// Purge removes the whole content of the Draft of a deleted Project
	// (ADR 0006), with no DraftChange: the cleanup writes as the server, so
	// the EditLock check of the transaction passes.
	Purge(ctx context.Context, scope editlock.Scope) (Draft, error)

	// ClearStack drops every DraftChange of the Draft and resets its stack
	// position: what a successful Deploy or Release leaves behind (ADR 0017).
	ClearStack(ctx context.Context, scope editlock.Scope) error
}
