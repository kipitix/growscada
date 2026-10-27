package apiv0

import (
	"time"

	"github.com/google/uuid"
)

// LibrariesQueryName is the filter of GET /libraries selecting the library
// with this exact name, unique among the server's live libraries.
const LibrariesQueryName = "name"

// LibraryResponse is the HTTP DTO for representing a library in API
// responses. The WidgetTypes of its Draft live under
// /libraries/{id}/widget-types, its releases under /libraries/{id}/releases.
// LatestRelease is the number of its latest release, 0 if none.
// DiscardingTo is the release a Discard under way resets the Draft to,
// absent when none is: until it ends the Draft is read-only.
// HasUnreleasedChanges reports whether the Draft differs from the latest
// release (always, for a library never released).
type LibraryResponse struct {
	ID                   uuid.UUID `json:"id"`
	Name                 string    `json:"name"`
	Version              int       `json:"version"`
	LatestRelease        int       `json:"latest_release"`
	DiscardingTo         int       `json:"discarding_to,omitempty"`
	HasUnreleasedChanges bool      `json:"has_unreleased_changes"`
}

// GetLibrariesResponse is the HTTP DTO for a list of libraries.
type GetLibrariesResponse struct {
	Libraries []LibraryResponse `json:"libraries"`
}

// CreateLibraryRequest is the HTTP DTO for creating a library.
type CreateLibraryRequest struct {
	Name string `json:"name"`
}

// CreateLibraryResponse is the HTTP DTO for a library creation response.
// Version is the new library's version, the one its first rename or
// release states.
type CreateLibraryResponse struct {
	ID      uuid.UUID `json:"id"`
	Version int       `json:"version"`
}

// UpdateLibraryRequest is the HTTP DTO for renaming a library.
// Version must match the current persisted version for optimistic locking.
type UpdateLibraryRequest struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// UpdateLibraryResponse is the HTTP DTO for a library rename response.
type UpdateLibraryResponse struct {
	Version int `json:"version"`
}

// CreateReleaseRequest is the HTTP DTO of POST /libraries/{id}/releases: a
// Release of the library's Draft as it is, with an optional note. Version
// must be the library's current version.
type CreateReleaseRequest struct {
	Note    string `json:"note"`
	Version int    `json:"version"`
}

// LibraryReleaseResponse is the HTTP DTO of a Library Release: its number
// within the library, the time it was made, its note and the copies of the
// WidgetTypes of the Draft it holds.
type LibraryReleaseResponse struct {
	LibraryID   uuid.UUID                    `json:"library_id"`
	Number      int                          `json:"number"`
	Note        string                       `json:"note"`
	CreatedAt   time.Time                    `json:"created_at"`
	WidgetTypes []ReleasedWidgetTypeResponse `json:"widget_types"`
}

// GetLibraryReleasesResponse is the HTTP DTO for the releases of a library,
// oldest first.
type GetLibraryReleasesResponse struct {
	Releases []LibraryReleaseResponse `json:"releases"`
}
