package settings

import (
	"context"
	"errors"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// ErrSettingsConflict is returned by Save when the settings were written by
// someone else since they were read: the stated Version is stale. A client
// rereads them and retries.
var ErrSettingsConflict = errors.New("user settings were changed concurrently")

// Repository keeps the per-user settings (task 81, ADR 0024). They are stored
// by their owner and Organization, apart from the aggregates they name; a
// write of them is one transaction over one row.
//
// The methods name the setting they keep — the library order — rather than
// hiding a key behind Find and Save: a second setting is a second pair of
// methods with its own row, never a key the caller has to know (ADR 0024).
type Repository interface {
	// FindLibraryOrder returns the library order of owner in org. ok is false
	// when the person never wrote one: the caller then works on Empty.
	FindLibraryOrder(ctx context.Context, owner id.ID[User], org id.ID[organization.Organization]) (Settings, bool, error)

	// SaveLibraryOrder writes the library order: one of the initial Version is
	// inserted at the first stored one, a stored one is replaced only while it
	// is still at the Version it was read at. A stale Version is
	// ErrSettingsConflict.
	SaveLibraryOrder(ctx context.Context, s Settings) (Settings, error)
}
