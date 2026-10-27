package library

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

var (
	// ErrReleaseNotFound is returned when a Library has no release with the
	// given number.
	ErrReleaseNotFound = errors.New("library release not found")
	// ErrReleaseNumberTaken is returned by Save when the Library already has
	// a release with the number: another Release won the race for it.
	ErrReleaseNumberTaken = errors.New("library release number already taken")
)

// ReleaseState is what a Library's releases tell of it: its latest release
// (the zero ReleaseNumber for none) and whether its Draft differs from it,
// as DraftDiffers decides.
type ReleaseState struct {
	Latest       ReleaseNumber
	DraftDiffers bool
}

// LibraryReleaseRepository - repository interface for storing and reading
// Library Releases. A release is only ever added, never changed nor removed
// but with its Library.
type LibraryReleaseRepository interface {
	// Save stores a new release and the events it recorded, in one
	// transaction, while its Library is live, still at the expected Version
	// and no Discard of it is under way.
	// Returns ErrLibraryNotFound if the Library is not live,
	// ErrLibraryConflict if it is not at expected,
	// ErrLibraryDiscarding while a Discard is under way,
	// ErrReleaseNumberTaken if the Library already has a release of its
	// number.
	Save(ctx context.Context, release LibraryRelease, expected version.Version[Library]) (LibraryRelease, error)

	// FindLatest returns the latest release of the Library, nil if it has
	// none.
	FindLatest(context.Context, id.ID[Library]) (LibraryRelease, error)

	// FindReleaseStates returns the ReleaseState of each of the Libraries,
	// read at once and without the WidgetTypes' content: the same as
	// DraftDiffers of its Draft and FindLatest. A Library that does not
	// exist has no release and a Draft that differs.
	FindReleaseStates(context.Context, []id.ID[Library]) (map[id.ID[Library]]ReleaseState, error)

	// FindByNumber returns the release of the Library with the number.
	// Returns ErrReleaseNotFound if there is none.
	FindByNumber(context.Context, id.ID[Library], ReleaseNumber) (LibraryRelease, error)

	// FindAll returns every release of the Library, oldest first.
	FindAll(context.Context, id.ID[Library]) ([]LibraryRelease, error)
}
