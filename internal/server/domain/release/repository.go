package release

import (
	"context"

	"github.com/kipitix/growscada/internal/server/domain/draft"
)

// Repository stores the Releases of the Engineering node's schema. A Release
// is stored with the copy of what it published, so it is read on its own: it
// names no Commit, Draft or Library row (task 72_3; ADR 0021).
type Repository interface {
	// Create publishes commit as the next Release of scope, with the note and
	// the Author: it locks the scope, numbers the Release after the latest one,
	// refuses a Commit that is already released (ErrCommitAlreadyReleased) and
	// writes the copy of the content with the Library Pins of a Project's
	// ProjectFile, in one transaction. A Project that is not live is
	// ErrProjectNotFound.
	Create(ctx context.Context, scope draft.Scope, commit draft.Commit, note Note, author draft.Author) (Release, error)

	// CreateFromFile publishes content as the next Release of scope with
	// origin file (task 72_4): no Commit is named, the title stands in its
	// place. It takes the same lock and number as Create, and writes the same
	// copy and pins, in one transaction.
	CreateFromFile(ctx context.Context, scope draft.Scope, title draft.CommitTitle, note Note,
		author draft.Author, content draft.CommitContent) (Release, error)

	// List returns the Releases of scope, newest first, without their content
	// in the DTO (the content is read for the counts).
	List(ctx context.Context, scope draft.Scope) ([]Release, error)

	// Find returns the Release number of scope. A number the scope does not
	// have is ErrReleaseNotFound; a scope that is not live is its own
	// not-found.
	Find(ctx context.Context, scope draft.Scope, number int) (Release, error)

	// UpdateNote changes the note of the Release number of scope and records
	// release_note_updated. A note is metadata: it states no Draft Version.
	// Returns ErrReleaseNotFound for a number the scope does not have.
	UpdateNote(ctx context.Context, scope draft.Scope, number int, note Note) (Release, error)

	// ReleasedCommits returns the numbers of the Commits the scope published.
	// It is empty for a scope with no Release, and for a Library's until
	// task 72_5.
	ReleasedCommits(ctx context.Context, scope draft.Scope) ([]int, error)
}
