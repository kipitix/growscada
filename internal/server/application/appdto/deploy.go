package appdto

import "github.com/google/uuid"

// DeployPreview is what a Deploy of a Project's Draft would do (task 35, Q9):
// the Deployment the Project runs now (absent until its first Deploy), whether
// the Draft is that Deployment unchanged, the Widgets of the running
// Deployment a Deploy would drop, and the Library Releases that Deployment
// names but this server does not have (task 71).
//
// DroppedWidgets is always empty today: a Deploy keeps every runtime entity
// until runtime Scenes and Widgets exist (task 48). The shape is fixed here,
// with the API that carries it, so that task adds no field.
type DeployPreview struct {
	CurrentDeployment *Deployment
	Unchanged         bool
	DroppedWidgets    []DroppedWidget
	// MissingReleases are the pins of the running Deployment's ProjectFile
	// whose Library or Release is gone from this server: a Discard drops their
	// Widgets instead of restoring them (task 71).
	MissingReleases []MissingRelease
}

// DroppedWidget names a Widget of the running Deployment that a Deploy would
// drop because its Scene or its WidgetType is gone from the Draft (task 35,
// Q15; task 48 fills it).
type DroppedWidget struct {
	SceneID  uuid.UUID
	WidgetID uuid.UUID
	Name     string
}

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
