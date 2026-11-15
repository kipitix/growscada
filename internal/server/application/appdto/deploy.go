package appdto

import "github.com/google/uuid"

// MissingImport names a saved state of a Library the running Deployment uses
// but this server does not have (task 71, task 85_6): the Document was deployed
// from a file, or its Library was removed after the Deployment was made.
type MissingImport struct {
	LibraryID uuid.UUID
	Commit    int
}

// DiscardWarning is one Library Import a Discard could not restore (task 71):
// the Library is gone, or the saved state is, and the Draft is left without
// that import. DroppedWidgets counts the Widgets of that Library's WidgetTypes
// that went with it — the Draft cannot hold them without the import.
type DiscardWarning struct {
	LibraryID      uuid.UUID
	Commit         int
	DroppedWidgets int
}

// DiscardedDraft is a Project's Draft after a Discard: its Version and the
// Library Imports the Discard could not restore.
type DiscardedDraft struct {
	Version  int
	Warnings []DiscardWarning
}

// LoadedDraft is a Draft after the content of one of its saved states replaced
// it (task 84): the new Version, whether the Draft then differs from its last
// saved state, and the Library Imports the loaded content used but this server
// does not have, whose imports and Widgets were dropped (a Project's only, task
// 71).
type LoadedDraft struct {
	Version           int
	HasUnsavedChanges bool
	Warnings          []DiscardWarning
}
