package apiv0

import (
	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// DeploymentCause is why a Deployment exists (task 36, ADR 0014; task 39,
// ADR 0018; task 72_4): a Deploy made it from a saved state, a Rollback copied
// an earlier one, or a project file was deployed as it is.
type DeploymentCause string

const (
	DeploymentCauseDeploy   DeploymentCause = "deploy"
	DeploymentCauseRollback DeploymentCause = "rollback"
	DeploymentCauseFile     DeploymentCause = "file"
)

// DeploymentResponse is the Deployment a Project runs: its number, when it was
// deployed, why it exists, the saved state it came from and the ProjectFile it
// holds (task 35, task 85_4). GET /projects/{id}/deployment answers 404 until
// the Project's first Deploy. RolledBackFrom names the Deployment a Rollback
// copied and is absent for a Deploy. SavedStateNumber is the saved state the
// Deployment came from (nil for a Deployment from a file, or one made before
// saved states were deployed). Nothing of the Draft is part of it: Operation
// reads this document alone.
type DeploymentResponse struct {
	Number           int                   `json:"number"`
	DeployedAt       string                `json:"deployed_at"`
	Cause            DeploymentCause       `json:"cause"`
	RolledBackFrom   *int                  `json:"rolled_back_from,omitempty"`
	SavedStateNumber *int                  `json:"saved_state_number,omitempty"`
	Content          projectv0.ProjectFile `json:"content"`
}

// DeploymentSummaryResponse names a Deployment without carrying its document: the
// number, the time it was deployed, why it exists and the saved state it came
// from (task 85_4).
type DeploymentSummaryResponse struct {
	Number           int             `json:"number"`
	DeployedAt       string          `json:"deployed_at"`
	Cause            DeploymentCause `json:"cause"`
	RolledBackFrom   *int            `json:"rolled_back_from,omitempty"`
	SavedStateNumber *int            `json:"saved_state_number,omitempty"`
}

// ListDeploymentsResponse is the Project's Deployments, newest first (task 36). A
// Project without a Deployment answers an empty list, not 404.
type ListDeploymentsResponse struct {
	Deployments []DeploymentSummaryResponse `json:"deployments"`
}

// DeployResponse is the answer of POST /projects/{id}/saved-states/{n}/deploy and
// of a deploy of a project file: the Deployment the Deploy made (task 85_4).
type DeployResponse struct {
	Number     int    `json:"number"`
	DeployedAt string `json:"deployed_at"`
}

// RevertToDeployedPath, under a Project's path, resets the Draft to the
// Deployment the Project runs (task 72_4). Discard resets it to the last
// saved state instead.
const RevertToDeployedPath = "/revert-to-deployed"

// DraftPath is the path, under a Project's path, of its Draft's state: the
// Version every content DTO of the Project carries and every content write
// states (ADR 0016).
const DraftPath = "/draft"

// DraftStateResponse is the state of a Project's or a Library's Draft: its
// Version and whether it holds changes that none of its saved states holds (ADR
// 0016, task 74). GET /projects/{id}/draft and GET /libraries/{id}/draft answer it,
// and so do POST /projects/{id}/discard (the reset to the last saved state),
// POST /projects/{id}/revert-to-deployed (the reset to the Deployment the
// Project runs) and POST /libraries/{id}/discard (to the last saved state)
// after the reset.
//
// In Problem.draft (409 draft_conflict) only Version is meaningful: the
// conflict names the Version the server holds, not the state of its flag.
type DraftStateResponse struct {
	Version           int  `json:"version"`
	HasUnsavedChanges bool `json:"has_unsaved_changes"`
}

// DiscardProjectResponse is the answer of POST /projects/{id}/discard — the
// Draft reset to its last saved state — and of
// POST /projects/{id}/revert-to-deployed — the Draft reset to the Deployment
// the Project runs —, each in one transaction (ADR 0016, task 72_4). Warnings
// name the saved states of Libraries the restored target used but this server
// does not have: their imports, and the Widgets of their WidgetTypes, were
// dropped from the restored Draft (task 71).
type DiscardProjectResponse struct {
	Version  int                      `json:"version"`
	Warnings []DiscardWarningResponse `json:"warnings,omitempty"`
}

// DiscardWarningResponse is one Library Import a Discard or a Revert could not
// restore: the Library is gone, or the saved state is. DroppedWidgets counts the
// Widgets of that Library's WidgetTypes that went with it (task 85_6).
type DiscardWarningResponse struct {
	LibraryID      uuid.UUID `json:"library_id"`
	SavedState     int       `json:"saved_state"`
	DroppedWidgets int       `json:"dropped_widgets"`
}

// DraftChangesPath is the path, under a Project's and a Library's path, of the
// state of the Draft's DraftChange stack (ADR 0017): what an undo and a redo
// would apply and the descriptions of those steps.
const DraftChangesPath = "/draft-changes"

// DraftChangesUndoPath and DraftChangesRedoPath are the commands of the stack:
// each applies one DraftChange and answers the state after it. Both state the
// Version of the Draft the client saw.
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
// Deployment, by Tag ID, for the first render of a reader that does not follow
// the Journal: later changes arrive as tag_updated. Operation reads its
// starting state from GET …/operation and follows the Journal instead (ADR
// 0020). A Project without a Deployment answers 404; a Tag of the Deployment
// without a state is left out (its state is the TagType's zero value with
// quality bad).
type GetTagStatesResponse struct {
	TagStates map[string]TagStateResponse `json:"tag_states"`
}
