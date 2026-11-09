package apiv0

import (
	"time"

	"github.com/google/uuid"
)

// LibrariesQueryName is the filter of GET /libraries selecting the library
// with this exact name, unique among the server's live libraries.
const LibrariesQueryName = "name"

// The origins of a Library Release (task 72_5): a Commit of the Library's
// Draft, or the Draft as it was before Commits (a legacy Release).
const (
	LibraryReleaseOriginCommit = "commit"
	LibraryReleaseOriginLegacy = "legacy"
)

// RevertToReleasePath, under a Library's path, resets its Draft to its latest
// Release (task 72_5): the published state. Discard resets it to the last
// Commit instead — the saved state.
const RevertToReleasePath = "/revert-to-release"

// LibraryResponse is the HTTP DTO for representing a library in API
// responses. The WidgetTypes of its Draft live under
// /libraries/{id}/widget-types, its releases under /libraries/{id}/releases.
// Description is the library's optional, free-text description of at most 500
// characters (task 73); it is an attribute of the library, never content.
// LatestRelease is the number of its latest release, 0 if none; LatestCommit
// the number of the Draft's last Commit, 0 when it was never saved.
// HasUnsavedChanges reports whether the Draft moved since its last Commit;
// HasUnpublishedChanges whether the last Commit is newer than the one the
// latest Release published (task 72_5; ADR 0021). Version is the Library's own
// (update and deletion); the Version of its Draft is read from GET
// /libraries/{id}/draft and carried by the DTOs of its content.
type LibraryResponse struct {
	ID                    uuid.UUID `json:"id"`
	Name                  string    `json:"name"`
	Description           string    `json:"description"`
	Version               int       `json:"version"`
	LatestRelease         int       `json:"latest_release"`
	LatestCommit          int       `json:"latest_commit"`
	HasUnsavedChanges     bool      `json:"has_unsaved_changes"`
	HasUnpublishedChanges bool      `json:"has_unpublished_changes"`
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
// Version is the new library's version, the one its first rename or
// release states.
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

// CreateReleaseRequest is the HTTP DTO of POST /libraries/{id}/releases: the
// publication of a Commit of the library's Draft as the next Library Release,
// with an optional note and the Author who published it (task 72_5;
// ADR 0021). It states no Draft Version: a Commit is immutable.
type CreateReleaseRequest struct {
	Commit int    `json:"commit"`
	Note   string `json:"note,omitempty"`
	Author string `json:"author"`
}

// UpdateLibraryReleaseNoteRequest is the HTTP DTO of
// PATCH /libraries/{id}/releases/{n}: the new note of a Library Release. The
// content never changes.
type UpdateLibraryReleaseNoteRequest struct {
	Note string `json:"note"`
}

// DiscardLibraryResponse is the answer of POST /libraries/{id}/discard: the
// Draft of the library was reset to its last Commit in one transaction
// (task 72_5; ADR 0016).
type DiscardLibraryResponse = DraftStateResponse

// RevertLibraryResponse is the answer of POST
// /libraries/{id}/revert-to-release: the Draft of the library was reset to its
// latest Release in one transaction (task 72_5; ADR 0016).
type RevertLibraryResponse = DraftStateResponse

// LibraryReleaseResponse is the HTTP DTO of a Library Release: its number
// within the library, the Commit it published and that Commit's title at the
// moment of the read, the Author, the origin (a Commit, or a legacy Release
// made before Commits), the note, the time, the copies of the WidgetTypes it
// holds and the counts of its snapshot. CommitNumber and Title are null for a
// legacy Release.
type LibraryReleaseResponse struct {
	LibraryID     uuid.UUID                    `json:"library_id"`
	Number        int                          `json:"number"`
	CommitNumber  *int                         `json:"commit_number"`
	Title         *string                      `json:"title"`
	Author        string                       `json:"author"`
	Origin        string                       `json:"origin"`
	Note          string                       `json:"note"`
	CreatedAt     time.Time                    `json:"created_at"`
	WidgetTypes   []ReleasedWidgetTypeResponse `json:"widget_types"`
	ContentCounts CommitContentCounts          `json:"content_counts"`
}

// GetLibraryReleasesResponse is the HTTP DTO for the releases of a library,
// oldest first.
type GetLibraryReleasesResponse struct {
	Releases []LibraryReleaseResponse `json:"releases"`
}
