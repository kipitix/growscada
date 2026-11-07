package appdto

import "github.com/google/uuid"

// DeployPreview is what a Deploy of a Project's Draft would do (task 35, Q9):
// the Deployment the Project runs now (absent until its first Deploy), whether
// the Draft is that Deployment unchanged, and the Widgets of the running
// Deployment a Deploy would drop.
//
// DroppedWidgets is always empty today: a Deploy keeps every runtime entity
// until runtime Scenes and Widgets exist (task 48). The shape is fixed here,
// with the API that carries it, so that task adds no field.
type DeployPreview struct {
	CurrentDeployment *Deployment
	Unchanged         bool
	DroppedWidgets    []DroppedWidget
}

// DroppedWidget names a Widget of the running Deployment that a Deploy would
// drop because its Scene or its WidgetType is gone from the Draft (task 35,
// Q15; task 48 fills it).
type DroppedWidget struct {
	SceneID  uuid.UUID
	WidgetID uuid.UUID
	Name     string
}
