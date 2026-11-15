package apiv0

import (
	"strconv"
	"time"

	"github.com/google/uuid"
)

// SavedStatePath is the path, under a Project's or a Library's path, of its
// saved states: the immutable snapshots a Save fixes (task 72_2; ADR 0021).
const SavedStatePath = "/saved-states"

// SavedStatePathOf is the path of the saved state number of a Project or
// Library.
func SavedStatePathOf(number int) string { return SavedStatePath + "/" + strconv.Itoa(number) }

// SavedStateLoadPath is the command, under SavedStatePathOf, that makes the
// Draft the content of that saved state (task 84): the saved state becomes what
// is edited, the DraftChange stack is cleared and the Version raised, as a
// Discard does. It states no Version — it is a reset of the Draft, not a write
// of its content (ADR 0016).
const SavedStateLoadPath = "/load"

// SavedStateLoadPathOf is the path of the command that loads the saved state
// number of a Project or Library into its Draft.
func SavedStateLoadPathOf(number int) string {
	return SavedStatePathOf(number) + SavedStateLoadPath
}

// SavedStateDeployPath is the command, under SavedStatePathOf, that makes the
// saved state the Project's running Deployment (task 85_4): the Deployment is
// the saved state's ProjectFile, and the Draft is not read.
const SavedStateDeployPath = "/deploy"

// SavedStateDeployPathOf is the path of the command that deploys the saved state
// number of a Project.
func SavedStateDeployPathOf(number int) string {
	return SavedStatePathOf(number) + SavedStateDeployPath
}

// SavedStateLabelsPath is the path, under SavedStatePathOf, of the labels a
// saved state carries (task 85_3; ADR 0025): PUT places one label on the saved
// state and DELETE takes it away, each idempotent and each stating no Draft
// Version.
const SavedStateLabelsPath = "/labels"

// SavedStateLabelPathOf is the path of the label labelID of a saved state.
func SavedStateLabelPathOf(number int, labelID string) string {
	return SavedStatePathOf(number) + SavedStateLabelsPath + "/" + labelID
}

// SavedStatesQueryLabel is the parameter of GET …/saved-states that keeps only
// the saved states the label with this ID marks (task 85_3). Without it the
// list is every saved state; a value that is not a UUID, or the zero UUID,
// which names no label, is invalid input.
const SavedStatesQueryLabel = "label"

// SavedStateLabelsResponse is the answer of PUT and DELETE of one label of a
// saved state: every label the saved state carries afterwards, ordered by name
// ignoring case (task 85_3; ADR 0025).
type SavedStateLabelsResponse struct {
	Labels []LabelResponse `json:"labels"`
}

// LoadSavedStateResponse is the answer of POST …/saved-states/{n}/load: the
// Draft after the saved state's content replaced it, in one transaction.
//
// HasUnsavedChanges is true unless the loaded saved state is the Draft's last
// one: the Draft then differs from what a Save would fix. Warnings name the
// saved states of Libraries the loaded content used but this server does not
// have — their imports and the Widgets of their WidgetTypes were dropped from
// the Draft, which stays writable (task 71); a Library's answer never carries
// one.
type LoadSavedStateResponse struct {
	Version           int                      `json:"version"`
	HasUnsavedChanges bool                     `json:"has_unsaved_changes"`
	Warnings          []DiscardWarningResponse `json:"warnings,omitempty"`
}

// CreateSavedStateRequest is the HTTP DTO of POST …/saved-states: a Save that
// fixes the Draft at version as a saved state with a title and the Author who
// made it.
type CreateSavedStateRequest struct {
	// Version is the Version of the Draft the client saw; a stale one is 409
	// draft_conflict, and one equal to the last saved state's is 409
	// nothing_to_save.
	Version int    `json:"version"`
	Title   string `json:"title"`
	Author  string `json:"author"`
}

// RenameSavedStateRequest is the HTTP DTO of PATCH …/saved-states/{n}: the new
// title of a saved state. It states no Draft Version: a title is not content.
type RenameSavedStateRequest struct {
	Title string `json:"title"`
}

// SavedStateContentCounts is the size of a saved state's snapshot: Scenes,
// Widgets, Tags, Devices and Environments of a Project's ProjectFile,
// WidgetTypes of a Library's Draft. The fields of the other kind are zero.
type SavedStateContentCounts struct {
	Scenes       int `json:"scenes"`
	Widgets      int `json:"widgets"`
	Tags         int `json:"tags"`
	Devices      int `json:"devices"`
	Environments int `json:"environments"`
	WidgetTypes  int `json:"widget_types"`
}

// SavedStateResponse is the HTTP DTO of a saved state without its content: its
// number within the Draft, its title, its author, the time it was made, the
// Version of the Draft it fixed and the counts of its snapshot (task 85_7).
// WidgetTypes is filled for the saved state of a Library alone, by
// GET …/saved-states/{n}; a Project's saved state is read as a ProjectFile from
// …/saved-states/{n}/projectfile.
type SavedStateResponse struct {
	ID            uuid.UUID                `json:"id"`
	Number        int                      `json:"number"`
	Title         string                   `json:"title"`
	Author        string                   `json:"author"`
	CreatedAt     time.Time                `json:"created_at"`
	DraftVersion  int                      `json:"draft_version"`
	ContentCounts SavedStateContentCounts  `json:"content_counts"`
	WidgetTypes   []WidgetTypeCopyResponse `json:"widget_types,omitempty"`
	// Labels are the labels that mark this saved state, ordered by name
	// ignoring case; absent when it carries none (task 85_3; ADR 0025).
	Labels []LabelResponse `json:"labels,omitempty"`
}

// GetSavedStatesResponse is the HTTP DTO of GET …/saved-states: newest first,
// without content.
type GetSavedStatesResponse struct {
	SavedStates []SavedStateResponse `json:"saved_states"`
}
