package apiv0

import (
	"time"

	"github.com/google/uuid"
)

// LibraryImportImpactQueryCommit is the parameter of GET
// /projects/{id}/library-imports/{library_id}/impact naming the saved state to
// preview importing; without it the preview is of removing the import.
const LibraryImportImpactQueryCommit = "commit"

// LibraryImportResponse is the HTTP DTO of a Library Import: the saved state —
// the Commit — of the Library the project uses, with its title, Author, time,
// labels and the Version of the Project's Draft it was read at (task 85_6).
type LibraryImportResponse struct {
	LibraryID    uuid.UUID       `json:"library_id"`
	CommitNumber int             `json:"commit_number"`
	Title        string          `json:"title"`
	Author       string          `json:"author"`
	SavedAt      time.Time       `json:"saved_at"`
	Labels       []LabelResponse `json:"labels,omitempty"`
	Version      int             `json:"version"`
}

// GetLibraryImportsResponse is the HTTP DTO for the imports of a project, in
// the order they were added.
type GetLibraryImportsResponse struct {
	LibraryImports []LibraryImportResponse `json:"library_imports"`
}

// PutLibraryImportRequest is the HTTP DTO of PUT
// /projects/{id}/library-imports/{library_id}: import the saved state numbered
// Commit. A Project that imports no saved state of the library gets the
// import, one that imports another switches it; Version is the Version of the
// Project's Draft the client saw (ADR 0016).
type PutLibraryImportRequest struct {
	Commit  int `json:"commit"`
	Version int `json:"version"`
}

// PinnedWidgetResponse names a Widget of the project: its Scene, its name and
// the WidgetType it is of.
type PinnedWidgetResponse struct {
	SceneID  uuid.UUID `json:"scene_id"`
	WidgetID uuid.UUID `json:"widget_id"`
	Name     string    `json:"name"`
	TypeID   uuid.UUID `json:"type_id"`
}

// BrokenBindingResponse is a PortBinding a switch of an import would remove:
// its port is no longer declared, or its type hint no longer accepts the Tag.
type BrokenBindingResponse struct {
	PinnedWidgetResponse
	PortName string    `json:"port_name"`
	TagID    uuid.UUID `json:"tag_id"`
}

// ImportImpactResponse is the HTTP DTO of the preview of switching or removing
// an import. OrphanedWidgets would be left without their WidgetType: while
// there are any, the switch or removal is refused (409). BrokenBindings would
// be removed in the same change.
type ImportImpactResponse struct {
	OrphanedWidgets []PinnedWidgetResponse  `json:"orphaned_widgets"`
	BrokenBindings  []BrokenBindingResponse `json:"broken_bindings"`
}
