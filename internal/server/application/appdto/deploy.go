package appdto

import "github.com/google/uuid"

// DeployPreview is what a Deploy of a Project's Draft would do (task 35, Q9):
// the Revision the Project runs now (absent until its first Deploy), whether
// the Draft is that Revision unchanged, and the Widgets of the running
// Revision a Deploy would drop.
//
// DroppedWidgets is always empty today: a Deploy keeps every runtime entity
// until runtime Scenes and Widgets exist (task 48). The shape is fixed here,
// with the API that carries it, so that task adds no field.
type DeployPreview struct {
	CurrentRevision *Revision
	Unchanged       bool
	DroppedWidgets  []DroppedWidget
}

// DroppedWidget names a Widget of the running Revision that a Deploy would
// drop because its Scene or its WidgetType is gone from the Draft (task 35,
// Q15; task 48 fills it).
type DroppedWidget struct {
	SceneID  uuid.UUID
	WidgetID uuid.UUID
	Name     string
}

// DeployState is the Project mode's view of the deployment of a Project (task
// 35, Q6): the Revision the Engineering node's copy of the last ProjectFile
// names — absent until a Deploy saved one — and whether the Draft differs from
// that copy. Nothing reports a dirty flag: the Draft is compared byte for
// byte, canonicalized, with the copy.
type DeployState struct {
	Deployed *Revision
	Differs  bool
}
