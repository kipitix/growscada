// Package settings holds the per-user settings of the server: the way one
// person sees the server's content, never the content itself (task 81,
// ADR 0024). A setting is an aggregate of its own, stored apart from the
// aggregates it names and orders, so a change of it neither raises their
// versions nor waits for their transactions (ADR 0008). It records no event:
// nothing else reacts to a preference, and the server's shared event stream is
// no place for one person's view.
//
// The User aggregate does not exist yet (task 49): the owner of the settings
// is SingleUser until authentication exists (task 50), and its ID then comes
// from the request's principal.
package settings

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// User is what a settings owner is: the person whose view they are. The type
// marks the aggregate in the owner's ID; the User aggregate itself is task 49.
type User struct{}

// singleUser is the one person every request acts as until Users and
// authentication exist: a fixed ID the migration that adds the Users
// backfills the settings written under it to.
var singleUser = id.NewID(id.IDWithUUID[User](uuid.MustParse("f0000000-0000-4000-8000-000000000001")))

// SingleUser returns the owner of every settings write until authentication
// exists (task 50).
func SingleUser() id.ID[User] { return singleUser }

// Settings are the settings of one person in one Organization. They hold the
// library order for now (task 81); a new setting is a new method and a new
// stored key, with no migration and no change of this aggregate's shape
// (ADR 0024).
//
// Settings are immutable: a change returns a new Settings. They record no
// event and carry the Version the store gives them, stated by every write of
// them.
type Settings struct {
	owner   id.ID[User]
	org     id.ID[organization.Organization]
	order   Order
	version version.Version[Settings]
}

var _ fmt.Stringer = Settings{}

// Empty returns the settings of a person who has written none: no order, at
// the initial Version (a store inserts them on their first write).
func Empty(own id.ID[User], org id.ID[organization.Organization]) Settings {
	return Settings{owner: own, org: org}
}

// Reconstitute rebuilds Settings as stored; it records no event. For
// repositories.
func Reconstitute(
	own id.ID[User],
	org id.ID[organization.Organization],
	order Order,
	aVersion version.Version[Settings],
) Settings {
	return Settings{owner: own, org: org, order: order, version: aVersion}
}

// Owner is the person the settings belong to; it never changes.
func (s Settings) Owner() id.ID[User] { return s.owner }

// OrganizationID is the tenant the view belongs to; it never changes.
func (s Settings) OrganizationID() id.ID[organization.Organization] { return s.org }

// LibraryOrder is the order the person sees the Organization's libraries in.
func (s Settings) LibraryOrder() Order { return s.order }

// Version is the Version the store holds the settings at; the zero Version
// means they were never written.
func (s Settings) Version() version.Version[Settings] { return s.version }

// WithLibraryOrder returns the settings with the libraries ordered as order
// places them. The Version is untouched — the store raises it, and refuses a
// write at a stale one — and no event is recorded (ADR 0024).
func (s Settings) WithLibraryOrder(order Order) Settings {
	changed := s
	changed.order = order
	return changed
}

// String implements [fmt.Stringer].
func (s Settings) String() string {
	return fmt.Sprintf("settings of user %s in organization %s at version %s: libraries %s",
		s.owner, s.org, s.version, s.order)
}
