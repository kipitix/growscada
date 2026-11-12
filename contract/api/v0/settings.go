package apiv0

import "github.com/google/uuid"

// LibraryOrderPath is the path, under PathPrefix, of the library order of the
// person the server acts for (task 81): a per-user setting, stored apart from
// the libraries it orders (ADR 0024).
const LibraryOrderPath = "/settings/library-order"

// LibraryOrderResponse is the HTTP DTO of a person's library order: the
// Version every write of it states, and the IDs of the libraries the person
// placed, first to last. It holds only the libraries they moved: the rest
// follow them, by name, and the client merges the two — a library the order
// does not name is not an error. Version 0 with an empty list is a person who
// never reordered, and Version 0 is also what their first write states.
type LibraryOrderResponse struct {
	Version    int         `json:"version"`
	LibraryIDs []uuid.UUID `json:"library_ids"`
}

// SetLibraryOrderRequest is the HTTP DTO of PUT /settings/library-order: the
// whole order the client shows and the Version it read (0 when there was
// none). LibraryIDs is required and always an array: an absent list is 400,
// and [] places no library. Repetitions are dropped and the IDs of libraries
// that do not exist are tolerated: an order is a view of the libraries, never
// an invariant about them (ADR 0024). A stale Version is 409, so a newer order
// is not overwritten silently.
type SetLibraryOrderRequest struct {
	Version    int         `json:"version"`
	LibraryIDs []uuid.UUID `json:"library_ids"`
}
