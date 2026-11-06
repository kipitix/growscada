package apiv0

import (
	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// RevisionCause is why a Revision exists (task 36, ADR 0014; task 39,
// ADR 0018): a Deploy made it from the Draft, a Rollback copied an earlier
// one, or a project file was deployed as it is.
type RevisionCause string

const (
	RevisionCauseDeploy   RevisionCause = "deploy"
	RevisionCauseRollback RevisionCause = "rollback"
	RevisionCauseFile     RevisionCause = "file"
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

// DraftPath is the path, under a Project's path, of its Draft's state: the
// Version every content DTO of the Project carries and every content write
// states (ADR 0016).
const DraftPath = "/draft"

// DraftStateResponse is the state of a Project's Draft: its Version. GET
// /projects/{id}/draft answers it, and POST /projects/{id}/discard answers it
// after the reset to the Revision the Project runs.
type DraftStateResponse struct {
	Version int `json:"version"`
}

// DiscardProjectResponse is the answer of POST /projects/{id}/discard: the
// Draft of the Project was reset to the Revision it runs in one transaction
// (ADR 0016).
type DiscardProjectResponse = DraftStateResponse

// DraftChangesPath is the path, under a Project's and a Library's path, of the
// state of the Draft's DraftChange stack (ADR 0017): what an undo and a redo
// would apply and the descriptions of those steps.
const DraftChangesPath = "/draft-changes"

// DraftChangesUndoPath and DraftChangesRedoPath are the commands of the stack:
// each applies one DraftChange and answers the state after it. Both state the
// Version of the Draft the client saw and are gated by its EditLock.
const (
	DraftChangesUndoPath = DraftChangesPath + "/undo"
	DraftChangesRedoPath = DraftChangesPath + "/redo"
)

// DraftChangesResponse is the state of a Draft's DraftChange stack. A
// description is absent exactly when its command has nothing to apply; an
// empty stack answers neither.
type DraftChangesResponse struct {
	Version         int    `json:"version"`
	CanUndo         bool   `json:"can_undo"`
	CanRedo         bool   `json:"can_redo"`
	UndoDescription string `json:"undo_description,omitempty"`
	RedoDescription string `json:"redo_description,omitempty"`
}

// DraftChangeCommandRequest is the body of POST …/draft-changes/undo and
// …/draft-changes/redo: the Version of the Draft the client saw. A stale one
// is 409, and so is an empty stack (nothing_to_undo, nothing_to_redo).
type DraftChangeCommandRequest struct {
	Version int `json:"version"`
}

// TagStateResponse is one Tag's state in the snapshot: its value, Quality,
// the Journal position of its latest change and when it was last written.
type TagStateResponse struct {
	Value     string  `json:"value"`
	Quality   string  `json:"quality"`
	Seq       int64   `json:"seq"`
	WrittenAt *string `json:"written_at,omitempty"`
}

// GetTagStatesResponse is the snapshot of the Tag states of the running
// Revision, by Tag ID, for the first render of Operation: later changes arrive
// as tag_updated. A Project without a Revision answers 404; a Tag of the
// Revision without a state is left out (its state is the TagType's zero value
// with quality bad).
type GetTagStatesResponse struct {
	TagStates map[string]TagStateResponse `json:"tag_states"`
}
