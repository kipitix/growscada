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
type Repository interface {
	// Find returns the settings of own in org. ok is false when the person
	// never wrote any: the caller then works on Empty.
	Find(ctx context.Context, own id.ID[User], org id.ID[organization.Organization]) (Settings, bool, error)

	// Save writes the settings: one of the initial Version is inserted at the
	// first stored one, a stored one is replaced only while it is still at the
	// Version it was read at. A stale Version is ErrSettingsConflict.
	Save(ctx context.Context, s Settings) (Settings, error)
}
