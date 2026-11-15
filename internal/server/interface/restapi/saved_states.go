package restapi

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// SavedStateHandlers serves the saved states of a Project's or a Library's Draft
// (task 72_2; ADR 0021): a Save fixes the Draft as a saved state, the history
// is listed (filtered by a label, task 85_3) and read, a title is renamed, a
// label is placed on a saved state or taken off it, and a Project's saved state
// is downloaded as the ProjectFile it holds. The same handler serves both
// paths; the scope is the path the request came under.
type SavedStateHandlers struct {
	savedStates application.SavedStateService
	labels      application.LabelService
}

// NewSavedStateHandler creates the handler over the Engineering node's saved states
// and over the labels of a saved state.
func NewSavedStateHandler(savedStates application.SavedStateService, labels application.LabelService) *SavedStateHandlers {
	return &SavedStateHandlers{savedStates: savedStates, labels: labels}
}

// scope returns the Draft scope the request names: a Library's when the
// library path parameter is present, the Project's otherwise.
func savedStateScope(w http.ResponseWriter, r *http.Request) (draft.Scope, bool) {
	if raw := r.PathValue(libraryResource.pathParam); raw != "" {
		libraryID, err := uuid.Parse(raw)
		if err != nil {
			sendProblem(w, NewBadRequest(fmt.Sprintf("%s: '%s' is not a valid UUID", libraryResource.pathParam, raw), r.URL.Path))
			return draft.Scope{}, false
		}
		return draft.LibraryScope(id.NewID(id.IDWithUUID[library.Library](libraryID))), true
	}
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return draft.Scope{}, false
	}
	return draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID))), true
}

// PostSavedState handles POST …/saved-states: a Save of the Draft at the stated
// Version. 201 with the saved state; 409 nothing_to_save, draft_not_saveable or
// draft_conflict; 400 for an empty title or author.
func (h SavedStateHandlers) PostSavedState(w http.ResponseWriter, r *http.Request) {
	scope, ok := savedStateScope(w, r)
	if !ok {
		return
	}
	var request apiv0.CreateSavedStateRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	savedState, err := h.savedStates.Save(r.Context(), scope, request.Version, request.Title, request.Author)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newSavedStateResponse(savedState))
}

// GetSavedStates handles GET …/saved-states: the history, newest first, without
// content; ?label=<id> keeps only the saved states that label marks
// (task 85_3).
func (h SavedStateHandlers) GetSavedStates(w http.ResponseWriter, r *http.Request) {
	scope, ok := savedStateScope(w, r)
	if !ok {
		return
	}
	labelID, ok := savedStatesLabelFilter(w, r)
	if !ok {
		return
	}
	list, err := h.savedStates.List(r.Context(), scope, labelID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetSavedStatesResponse(list))
}

// GetSavedStateByNumber handles GET …/saved-states/{saved_state_number}: one
// saved state, with the WidgetTypes of a Library's snapshot. 404 for a number
// the Draft does not have.
func (h SavedStateHandlers) GetSavedStateByNumber(w http.ResponseWriter, r *http.Request) {
	scope, ok := savedStateScope(w, r)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, savedStateResource)
	if !ok {
		return
	}
	savedState, err := h.savedStates.Find(r.Context(), scope, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newSavedStateResponse(savedState))
}

// PatchSavedState handles PATCH …/saved-states/{saved_state_number}: a new
// title for a saved state. It states no Draft Version.
func (h SavedStateHandlers) PatchSavedState(w http.ResponseWriter, r *http.Request) {
	scope, ok := savedStateScope(w, r)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, savedStateResource)
	if !ok {
		return
	}
	var request apiv0.RenameSavedStateRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	savedState, err := h.savedStates.Rename(r.Context(), scope, number, request.Title)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newSavedStateResponse(savedState))
}

// GetSavedStateProjectFile handles GET
// /projects/{project_id}/saved-states/{saved_state_number}/projectfile: the
// exact document the saved state fixed, as an attachment (as a Deployment's
// document is downloaded). 404 for a number the Project does not have.
func (h SavedStateHandlers) GetSavedStateProjectFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, savedStateResource)
	if !ok {
		return
	}
	scope := draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID)))
	download, err := h.savedStates.ProjectFile(r.Context(), scope, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendProjectFile(w, download.File, projectFileName(download.ProjectName, projectID, fmt.Sprintf("saved-state-%d", number)))
}

// PutSavedStateLabel handles PUT
// …/saved-states/{saved_state_number}/labels/{label_id}: the label marks the
// saved state. It is idempotent and states no Draft Version; the answer is
// every label the saved state carries afterwards (task 85_3; ADR 0025).
func (h SavedStateHandlers) PutSavedStateLabel(w http.ResponseWriter, r *http.Request) {
	scope, number, labelID, ok := savedStateLabelPath(w, r)
	if !ok {
		return
	}
	labels, err := h.labels.Assign(r.Context(), scope, number, labelID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newSavedStateLabelsResponse(labels))
}

// DeleteSavedStateLabel handles DELETE
// …/saved-states/{saved_state_number}/labels/{label_id}: the label no longer
// marks the saved state. A repeat is a 200 with the labels it still carries,
// never a 409 (task 85_3).
func (h SavedStateHandlers) DeleteSavedStateLabel(w http.ResponseWriter, r *http.Request) {
	scope, number, labelID, ok := savedStateLabelPath(w, r)
	if !ok {
		return
	}
	labels, err := h.labels.Unassign(r.Context(), scope, number, labelID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newSavedStateLabelsResponse(labels))
}

// savedStateLabelPath reads the scope, the saved state's number and the label
// of a route of a saved state's labels. On failure it has already answered.
func savedStateLabelPath(w http.ResponseWriter, r *http.Request) (draft.Scope, int, uuid.UUID, bool) {
	scope, ok := savedStateScope(w, r)
	if !ok {
		return draft.Scope{}, 0, uuid.Nil, false
	}
	number, ok := pathInt(w, r, savedStateResource)
	if !ok {
		return draft.Scope{}, 0, uuid.Nil, false
	}
	labelID, ok := pathUUID(w, r, labelResource)
	if !ok {
		return draft.Scope{}, 0, uuid.Nil, false
	}
	return scope, number, labelID, true
}

// savedStatesLabelFilter reads ?label=<uuid> of a saved state list: uuid.Nil
// (every saved state) when the parameter is absent; a value that is not a UUID,
// or the zero UUID, which names no label, is the client's input. On failure it
// has already answered.
func savedStatesLabelFilter(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := r.URL.Query().Get(apiv0.SavedStatesQueryLabel)
	if raw == "" {
		return uuid.Nil, true
	}
	labelID, err := uuid.Parse(raw)
	if err != nil {
		sendProblem(w, NewBadRequest(fmt.Sprintf("%s: '%s' is not a valid UUID", apiv0.SavedStatesQueryLabel, raw), r.URL.Path))
		return uuid.Nil, false
	}
	if labelID == uuid.Nil {
		// uuid.Nil is the filter's "every saved state": a client that names the
		// zero UUID asks for a label that cannot exist, and keeping every state
		// would answer the opposite of what it asked (task 85_3).
		sendProblem(w, NewBadRequest(fmt.Sprintf("%s: '%s' names no label", apiv0.SavedStatesQueryLabel, raw), r.URL.Path))
		return uuid.Nil, false
	}
	return labelID, true
}
