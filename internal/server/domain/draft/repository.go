package draft

import (
	"context"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/version"
)

// CommitFilter narrows the saved state list of a Draft. The zero filter lists
// every saved state.
type CommitFilter struct {
	// LabelID keeps only the saved states the label marks, filtered by the SQL
	// join over engineering.commit_labels (task 85_3; ADR 0025). uuid.Nil keeps
	// every saved state.
	LabelID uuid.UUID
}

// Repository stores the Drafts and their DraftChanges in the Engineering
// node's schema (ADR 0016, ADR 0017). One write of a Draft's content is one
// transaction over its row: the repository locks it, checks the Version the
// write states, runs the operation and writes the touched rows, the
// DraftChange, the new Version and stack position and the outbox's events.
type Repository interface {
	// Read returns the Draft of scope and whether a row exists; a Draft never
	// written has none. A Project or Library that does not exist has none
	// either.
	Read(ctx context.Context, scope Scope) (Draft, bool, error)

	// Update runs one write of the Draft's content: it locks the Draft's row,
	// checks expected — a stale Version is ErrDraftConflict — runs change,
	// writes the rows its snapshots name, its events and one DraftChange
	// described by description, raises the Version, appends the change at the
	// stack and commits. Returns the Draft as stored.
	Update(ctx context.Context, scope Scope, expected version.Version[Draft], description string, change func(*Session) error) (Draft, error)

	// CheckVersion locks the Draft's row and checks it is still at expected: a
	// stale Version is a ConflictError. It is the check a multi-step act over the
	// Draft makes after it read the Draft and before it acts on it (ADR 0022). A
	// Deploy takes a saved state and reads no Draft, so the pattern has no caller
	// since task 72_4; ADR 0022 keeps it for the next act that needs it.
	CheckVersion(ctx context.Context, scope Scope, expected version.Version[Draft]) error

	// Replace runs a write that replaces the whole content of the Draft
	// (Discard, Load, ADR 0016): it writes the rows change touched, clears the
	// DraftChanges and the stack and commits, with no DraftChange of its own.
	// cleanCommit is the number of the saved state whose content change writes, 0
	// when the content is no saved state's. The Clean Version becomes the new
	// Version exactly when that saved state is the Draft's last one — compared
	// under the Draft row's lock, so a Save committed between the caller's read
	// and this transaction cannot mark another content clean. Otherwise the
	// marker is untouched, so the raised Version stays above it and the Draft
	// reports unsaved changes (ADR 0016, amendment 2026-11-10). Returns the
	// Draft as stored.
	Replace(ctx context.Context, scope Scope, cleanCommit int, change func(*Session) error) (Draft, error)

	// Purge removes the whole content of the Draft of a deleted Project
	// (ADR 0006), with no DraftChange.
	Purge(ctx context.Context, scope Scope) (Draft, error)

	// ClearStack drops every DraftChange of the Draft and resets its stack
	// position: nothing is left to undo or redo, not even the changes made since
	// the last saved state. Replace clears the stack in the write it makes, so
	// no caller uses it since task 85_7.
	ClearStack(ctx context.Context, scope Scope) error

	// Stack returns the state of the Draft's DraftChange stack without a
	// write: the Draft, the change an undo would apply and the one a redo
	// would apply. ok is false for a Draft that was never written: it has no
	// row and no stack.
	Stack(ctx context.Context, scope Scope) (StackState, bool, error)

	// Undo applies the before states of the DraftChange at the stack's
	// position (ADR 0017): it locks the Draft's row, checks expected, writes
	// the rows, raises the Version, steps the position back and appends the
	// inverse events, with no DraftChange of its own. Returns ErrNothingToUndo
	// when the stack has no change there and a ConflictError for a stale
	// expected.
	Undo(ctx context.Context, scope Scope, expected version.Version[Draft]) (Draft, error)

	// Redo applies the after states of the DraftChange after the stack's
	// position, as Undo does its before states. Returns ErrNothingToRedo when
	// nothing was undone.
	Redo(ctx context.Context, scope Scope, expected version.Version[Draft]) (Draft, error)

	// SaveCommit fixes the Draft as a saved state (task 72_2; ADR 0021): it locks
	// the Draft's row, checks expected — a stale Version is a ConflictError —,
	// refuses a Draft at its Clean Version (ErrNothingToSave: there is nothing the
	// last saved state does not hold already), numbers the saved state after the
	// last one, writes it with the imports of its ProjectFile, raises
	// last_commit_number and last_commit_draft_version and makes expected the
	// Clean Version (ADR 0016, amendment 2026-11-10). content is assembled by the
	// caller from the Draft at expected (a ProjectFile or a Library's
	// WidgetTypes); no DraftChange is written and the Version is not raised.
	SaveCommit(ctx context.Context, scope Scope, expected version.Version[Draft], title CommitTitle, author Author, content CommitContent) (Commit, error)

	// RenameCommit changes the title of the saved state number of the Draft and
	// records commit_title_updated. It states no Draft Version: the title is
	// not content, so the command works after another Engineer's writes.
	// Returns ErrCommitNotFound for a number the Draft does not have.
	RenameCommit(ctx context.Context, scope Scope, number int, title CommitTitle) (Commit, error)

	// FindCommits returns the Draft's saved states, newest first, or only those
	// the filter's label marks. A Project or Library that is not live is
	// ErrProjectNotFound or ErrLibraryNotFound.
	FindCommits(ctx context.Context, scope Scope, filter CommitFilter) ([]Commit, error)

	// FindLastCommit returns the Draft's newest saved state, and whether it has
	// one at all: a Draft never saved has none. It is the read of a Discard, which
	// falls back to it (task 72_4) and needs no other saved state. A Project or
	// Library that is not live is ErrProjectNotFound or ErrLibraryNotFound.
	FindLastCommit(ctx context.Context, scope Scope) (Commit, bool, error)

	// FindCommit returns the saved state number of the Draft. A Project or Library
	// that is not live is ErrProjectNotFound or ErrLibraryNotFound; a number
	// the Draft does not have is ErrCommitNotFound.
	FindCommit(ctx context.Context, scope Scope, number int) (Commit, error)
}
