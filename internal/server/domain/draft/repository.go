package draft

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/version"
)

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
	// stale Version is a ConflictError. It is the check a multi-step act over
	// the Draft (Deploy, which builds the ProjectFile over several queries)
	// makes after it read the Draft and before it acts on it (ADR 0022).
	CheckVersion(ctx context.Context, scope Scope, expected version.Version[Draft]) error

	// Replace runs a write that replaces the whole content of the Draft
	// (Discard, ADR 0016): it writes the rows change touched, clears the
	// DraftChanges and the stack and commits, with no DraftChange of its own.
	// Returns the Draft as stored.
	Replace(ctx context.Context, scope Scope, change func(*Session) error) (Draft, error)

	// Purge removes the whole content of the Draft of a deleted Project
	// (ADR 0006), with no DraftChange.
	Purge(ctx context.Context, scope Scope) (Draft, error)

	// ClearStack drops every DraftChange of the Draft and resets its stack
	// position: what a successful Deploy or Release leaves behind (ADR 0017).
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

	// SaveCommit fixes the Draft as a Commit (task 72_2; ADR 0021): it locks
	// the Draft's row, checks expected — a stale Version is a ConflictError —,
	// refuses a Draft at the Version of its last Commit (ErrNothingToSave),
	// numbers the Commit after the last one, writes it with the pins of its
	// ProjectFile and raises last_commit_number and last_commit_draft_version.
	// content is assembled by the caller from the Draft at expected (a
	// ProjectFile or a Library's WidgetTypes); no DraftChange is written and
	// the Version is not raised.
	SaveCommit(ctx context.Context, scope Scope, expected version.Version[Draft], title CommitTitle, author Author, content CommitContent) (Commit, error)

	// RenameCommit changes the title of the Commit number of the Draft and
	// records commit_title_updated. It states no Draft Version: the title is
	// not content, so the command works after another Engineer's writes.
	// Returns ErrCommitNotFound for a number the Draft does not have.
	RenameCommit(ctx context.Context, scope Scope, number int, title CommitTitle) (Commit, error)

	// FindCommits returns the Draft's Commits, newest first. A Project or
	// Library that is not live is ErrProjectNotFound or ErrLibraryNotFound.
	FindCommits(ctx context.Context, scope Scope) ([]Commit, error)

	// FindCommit returns the Commit number of the Draft. A Project or Library
	// that is not live is ErrProjectNotFound or ErrLibraryNotFound; a number
	// the Draft does not have is ErrCommitNotFound.
	FindCommit(ctx context.Context, scope Scope, number int) (Commit, error)
}
