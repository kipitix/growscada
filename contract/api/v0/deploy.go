package apiv0

import (
	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// RevisionCause is why a Revision exists (task 36, ADR 0014): a Deploy made it
// from the Draft, or a Rollback copied an earlier one.
type RevisionCause string

const (
	RevisionCauseDeploy   RevisionCause = "deploy"
	RevisionCauseRollback RevisionCause = "rollback"
)

// RevisionResponse is the Revision a Project runs: its number, when it was
// deployed, why it exists and the ProjectFile it holds (task 35). GET
// /projects/{id}/revision answers 404 until the Project's first Deploy.
// RolledBackFrom names the Revision a Rollback copied and is absent for a
// Deploy. Nothing of the Draft is part of it: Operation reads this document
// alone.
type RevisionResponse struct {
	Number         int                   `json:"number"`
	DeployedAt     string                `json:"deployed_at"`
	Cause          RevisionCause         `json:"cause"`
	RolledBackFrom *int                  `json:"rolled_back_from,omitempty"`
	Content        projectv0.ProjectFile `json:"content"`
}

// RevisionSummaryResponse names a Revision without carrying its document: the
// number, the time it was deployed and why it exists.
type RevisionSummaryResponse struct {
	Number         int           `json:"number"`
	DeployedAt     string        `json:"deployed_at"`
	Cause          RevisionCause `json:"cause"`
	RolledBackFrom *int          `json:"rolled_back_from,omitempty"`
}

// ListRevisionsResponse is the Project's Revisions, newest first (task 36). A
// Project without a Revision answers an empty list, not 404.
type ListRevisionsResponse struct {
	Revisions []RevisionSummaryResponse `json:"revisions"`
}

// DroppedWidgetResponse names a Widget of the running Revision that a Deploy
// would drop, because its Scene or its WidgetType is gone from the Draft
// (task 48 fills the list; until then it is always empty).
type DroppedWidgetResponse struct {
	SceneID  uuid.UUID `json:"scene_id"`
	WidgetID uuid.UUID `json:"widget_id"`
	Name     string    `json:"name"`
}

// DeployPreviewResponse is what a Deploy of the Draft would do: the Revision
// the Project runs now (absent until its first Deploy), whether the Draft is
// that Revision unchanged, and the runtime Widgets a Deploy would drop.
type DeployPreviewResponse struct {
	CurrentRevision *RevisionSummaryResponse `json:"current_revision,omitempty"`
	Unchanged       bool                     `json:"unchanged"`
	DroppedWidgets  []DroppedWidgetResponse  `json:"dropped_widgets"`
}

// DeployResponse is the answer of POST /projects/{id}/deploy: the Revision the
// Deploy made.
type DeployResponse struct {
	Number     int    `json:"number"`
	DeployedAt string `json:"deployed_at"`
}

// DiscardProjectResponse is the answer of POST /projects/{id}/discard: the
// Project is being reset to the Revision it runs, its Draft read-only until
// the Discard ends.
type DiscardProjectResponse struct {
	Discarding bool `json:"discarding"`
}

// TagStateResponse is one Tag's state in the snapshot: its value, Quality and
// the seq of its latest write.
type TagStateResponse struct {
	Value   string `json:"value"`
	Quality string `json:"quality"`
	Seq     int64  `json:"seq"`
}

// GetTagStatesResponse is the snapshot of the Tag states of the running
// Revision, by Tag ID, for the first render of Operation: later changes arrive
// as tag_updated. A Project without a Revision answers 404; a Tag of the
// Revision without a state is left out (its state is the TagType's zero value
// with quality bad).
type GetTagStatesResponse struct {
	TagStates map[string]TagStateResponse `json:"tag_states"`
}
