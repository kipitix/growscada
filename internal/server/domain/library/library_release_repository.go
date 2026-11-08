package library

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

var (
	// ErrReleaseNotFound is returned when a Library has no release with the
	// given number.
	ErrReleaseNotFound = errors.New("library release not found")
	// ErrReleaseNumberTaken is returned by Save when the Library already has
	// a release with the number: another Release won the race for it.
	ErrReleaseNumberTaken = errors.New("library release number already taken")
	// ErrCommitAlreadyReleased is returned by Save when the Library already has
	// a Release of the Commit: one Commit is published at most once (task 72_5).
	ErrCommitAlreadyReleased = errors.New("library commit already released")
)

// LibraryReleaseRepository - repository interface for storing and reading
// Library Releases. A release is only ever added, never changed nor removed
// but with its Library.
type LibraryReleaseRepository interface {
	// Save stores a new release and the events it recorded, in one
	// transaction, while its Library is live. A Release made from a Commit is
	// stored with that Commit's number, its Author and its copies of the
	// WidgetTypes; a legacy one names no Commit.
	// Returns ErrLibraryNotFound if the Library is not live,
	// ErrReleaseNumberTaken if the Library already has a release of its
	// number, and ErrCommitAlreadyReleased if the Commit is published already.
	Save(ctx context.Context, release LibraryRelease) (LibraryRelease, error)

	// FindLatest returns the latest release of the Library, nil if it has
	// none.
	FindLatest(context.Context, id.ID[Library]) (LibraryRelease, error)

	// FindByNumber returns the release of the Library with the number.
	// Returns ErrReleaseNotFound if there is none.
	FindByNumber(context.Context, id.ID[Library], ReleaseNumber) (LibraryRelease, error)

	// FindAll returns every release of the Library, oldest first.
	FindAll(context.Context, id.ID[Library]) ([]LibraryRelease, error)

	// UpdateNote changes the note of the release of the Library and records
	// library_release_note_updated. A note is metadata: the content never
	// changes, and no Version is stated.
	// Returns ErrReleaseNotFound for a number the Library does not have.
	UpdateNote(ctx context.Context, libraryID id.ID[Library], number ReleaseNumber, note string) (LibraryRelease, error)

	// ReleasedCommits returns the numbers of the Commits the Library published
	// as Releases, oldest first. It is empty for a Library with no Release made
	// from a Commit.
	ReleasedCommits(ctx context.Context, libraryID id.ID[Library]) ([]int, error)
}
