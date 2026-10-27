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
)

// LibraryReleaseRepository - repository interface for storing and reading
// Library Releases. A release is only ever added, never changed nor removed
// but with its Library.
type LibraryReleaseRepository interface {
	// Save stores a new release and the events it recorded, in one
	// transaction, while its Library is live and no Discard of it is under
	// way.
	// Returns ErrLibraryNotFound if the Library is not live,
	// ErrLibraryDiscarding while a Discard is under way,
	// ErrReleaseNumberTaken if the Library already has a release of its
	// number.
	Save(context.Context, LibraryRelease) (LibraryRelease, error)

	// FindLatest returns the latest release of the Library, nil if it has
	// none.
	FindLatest(context.Context, id.ID[Library]) (LibraryRelease, error)

	// FindByNumber returns the release of the Library with the number.
	// Returns ErrReleaseNotFound if there is none.
	FindByNumber(context.Context, id.ID[Library], ReleaseNumber) (LibraryRelease, error)

	// FindAll returns every release of the Library, oldest first.
	FindAll(context.Context, id.ID[Library]) ([]LibraryRelease, error)
}
