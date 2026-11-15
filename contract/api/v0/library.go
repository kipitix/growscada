package apiv0

import (
	"github.com/google/uuid"
)

// LibrariesQueryName is the filter of GET /libraries selecting the library
// with this exact name, unique among the server's live libraries.
const LibrariesQueryName = "name"

// LibraryResponse is the HTTP DTO for representing a library in API
// responses. The WidgetTypes of its Draft live under
// /libraries/{id}/widget-types, its saved states under
// /libraries/{id}/saved-states. Description is the library's optional, free-text
// description of at most 500 characters (task 73); it is an attribute of the
// library, never content. LatestSavedState is the number of the Draft's last
// saved state, 0 when it was never saved; HasUnsavedChanges reports whether the
// Draft moved since that saved state (task 85_7). Version is the Library's own
// (update and deletion); the Version of its Draft is read from GET
// /libraries/{id}/draft and carried by the DTOs of its content.
type LibraryResponse struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Version           int       `json:"version"`
	LatestSavedState  int       `json:"latest_saved_state"`
	HasUnsavedChanges bool      `json:"has_unsaved_changes"`
}

// GetLibrariesResponse is the HTTP DTO for a list of libraries.
type GetLibrariesResponse struct {
	Libraries []LibraryResponse `json:"libraries"`
}

// CreateLibraryRequest is the HTTP DTO for creating a library.
type CreateLibraryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateLibraryResponse is the HTTP DTO for a library creation response.
// Version is the new library's version, the one its first rename states.
type CreateLibraryResponse struct {
	ID      uuid.UUID `json:"id"`
	Version int       `json:"version"`
}

// UpdateLibraryRequest is the HTTP DTO for updating a library's name and
// description. Version must match the current persisted version for optimistic
// locking. An absent Description keeps the current one, so a client that sends
// only a name does not erase it; an empty string clears it (task 73).
type UpdateLibraryRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Version     int     `json:"version"`
}

// UpdateLibraryResponse is the HTTP DTO for a library update response.
type UpdateLibraryResponse struct {
	Version int `json:"version"`
}

// DiscardLibraryResponse is the answer of POST /libraries/{id}/discard: the
// Draft of the library was reset to its last saved state in one transaction
// (task 72_5; ADR 0016).
type DiscardLibraryResponse = DraftStateResponse
