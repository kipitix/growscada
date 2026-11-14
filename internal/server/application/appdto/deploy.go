package appdto

import "github.com/google/uuid"

// MissingRelease names a Library Release the running Deployment uses but this
// server does not have (task 71): the Document was deployed from a file, or its
// Library was removed after the Deployment was made.
type MissingRelease struct {
	LibraryID uuid.UUID
	Release   int
}

// DiscardWarning is one Library Pin a Discard could not restore (task 71): the
// Library is gone, or the Release is, and the Draft is left without that pin.
// DroppedWidgets counts the Widgets of that Library's WidgetTypes that went
// with it — the Draft cannot hold them without the pin.
type DiscardWarning struct {
	LibraryID      uuid.UUID
	Release        int
	DroppedWidgets int
}

// DiscardedDraft is a Project's Draft after a Discard: its Version and the
// Library Releases the Discard could not restore.
type DiscardedDraft struct {
	Version  int
	Warnings []DiscardWarning
}

// LoadedDraft is a Draft after the content of one of its Commits replaced it
// (task 84): the new Version, whether the Draft then differs from its last
// Commit, and the Library Releases the loaded content used but this server does
// not have, whose pins and Widgets were dropped (a Project's only, task 71).
type LoadedDraft struct {
	Version           int
	HasUnsavedChanges bool
	Warnings          []DiscardWarning
}
