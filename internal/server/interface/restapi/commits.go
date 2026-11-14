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

// CommitHandlers serves the Commits of a Project's or a Library's Draft (task
// 72_2; ADR 0021): a Save fixes the Draft as a Commit, the history is listed
// (filtered by a label, task 85_3) and read, a title is renamed, a label is
// placed on a saved state or taken off it, and a Project's Commit is
// downloaded as the ProjectFile it holds. The same handler serves both paths;
// the scope is the path the request came under.
type CommitHandlers struct {
	commits application.CommitService
	labels  application.LabelService
}

// NewCommitHandler creates the handler over the Engineering node's Commits and
// over the labels of a saved state.
func NewCommitHandler(commits application.CommitService, labels application.LabelService) *CommitHandlers {
	return &CommitHandlers{commits: commits, labels: labels}
}

// scope returns the Draft scope the request names: a Library's when the
// library path parameter is present, the Project's otherwise.
func commitScope(w http.ResponseWriter, r *http.Request) (draft.Scope, bool) {
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

// PostCommit handles POST …/commits: a Save of the Draft at the stated
// Version. 201 with the Commit; 409 nothing_to_save, draft_not_saveable or
// draft_conflict; 400 for an empty title or author.
func (h CommitHandlers) PostCommit(w http.ResponseWriter, r *http.Request) {
	scope, ok := commitScope(w, r)
	if !ok {
		return
	}
	var request apiv0.CreateCommitRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	commit, err := h.commits.Save(r.Context(), scope, request.Version, request.Title, request.Author)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newCommitResponse(commit))
}

// GetCommits handles GET …/commits: the history, newest first, without
// content; ?label=<id> keeps only the saved states that label marks
// (task 85_3).
func (h CommitHandlers) GetCommits(w http.ResponseWriter, r *http.Request) {
	scope, ok := commitScope(w, r)
	if !ok {
		return
	}
	labelID, ok := commitsLabelFilter(w, r)
	if !ok {
		return
	}
	commits, err := h.commits.List(r.Context(), scope, labelID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetCommitsResponse(commits))
}

// GetCommitByNumber handles GET …/commits/{commit_number}: one Commit, with
// the WidgetTypes of a Library's snapshot. 404 for a number the Draft does
// not have.
func (h CommitHandlers) GetCommitByNumber(w http.ResponseWriter, r *http.Request) {
	scope, ok := commitScope(w, r)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, commitResource)
	if !ok {
		return
	}
	commit, err := h.commits.Find(r.Context(), scope, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newCommitResponse(commit))
}

// PatchCommit handles PATCH …/commits/{commit_number}: a new title for a
// Commit. It states no Draft Version.
func (h CommitHandlers) PatchCommit(w http.ResponseWriter, r *http.Request) {
	scope, ok := commitScope(w, r)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, commitResource)
	if !ok {
		return
	}
	var request apiv0.RenameCommitRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	commit, err := h.commits.Rename(r.Context(), scope, number, request.Title)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newCommitResponse(commit))
}

// GetCommitProjectFile handles GET
// /projects/{project_id}/commits/{commit_number}/projectfile: the exact
// document the Commit fixed, as an attachment (as a Deployment's document is
// downloaded). 404 for a number the Project does not have.
func (h CommitHandlers) GetCommitProjectFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, commitResource)
	if !ok {
		return
	}
	scope := draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID)))
	download, err := h.commits.ProjectFile(r.Context(), scope, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendProjectFile(w, download.File, projectFileName(download.ProjectName, projectID, fmt.Sprintf("commit-%d", number)))
}

// PutCommitLabel handles PUT …/commits/{commit_number}/labels/{label_id}: the
// label marks the saved state. It is idempotent and states no Draft Version;
// the answer is every label the saved state carries afterwards (task 85_3;
// ADR 0025).
func (h CommitHandlers) PutCommitLabel(w http.ResponseWriter, r *http.Request) {
	scope, number, labelID, ok := commitLabelPath(w, r)
	if !ok {
		return
	}
	labels, err := h.labels.Assign(r.Context(), scope, number, labelID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newCommitLabelsResponse(labels))
}

// DeleteCommitLabel handles DELETE …/commits/{commit_number}/labels/{label_id}:
// the label no longer marks the saved state. A repeat is a 200 with the labels
// it still carries, never a 409 (task 85_3).
func (h CommitHandlers) DeleteCommitLabel(w http.ResponseWriter, r *http.Request) {
	scope, number, labelID, ok := commitLabelPath(w, r)
	if !ok {
		return
	}
	labels, err := h.labels.Unassign(r.Context(), scope, number, labelID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newCommitLabelsResponse(labels))
}

// commitLabelPath reads the scope, the saved state's number and the label of a
// route of a saved state's labels. On failure it has already answered.
func commitLabelPath(w http.ResponseWriter, r *http.Request) (draft.Scope, int, uuid.UUID, bool) {
	scope, ok := commitScope(w, r)
	if !ok {
		return draft.Scope{}, 0, uuid.Nil, false
	}
	number, ok := pathInt(w, r, commitResource)
	if !ok {
		return draft.Scope{}, 0, uuid.Nil, false
	}
	labelID, ok := pathUUID(w, r, labelResource)
	if !ok {
		return draft.Scope{}, 0, uuid.Nil, false
	}
	return scope, number, labelID, true
}

// commitsLabelFilter reads ?label=<uuid> of a Commit list: uuid.Nil (every
// saved state) when the parameter is absent; a value that is not a UUID is the
// client's input. On failure it has already answered.
func commitsLabelFilter(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := r.URL.Query().Get(apiv0.CommitsQueryLabel)
	if raw == "" {
		return uuid.Nil, true
	}
	labelID, err := uuid.Parse(raw)
	if err != nil {
		sendProblem(w, NewBadRequest(fmt.Sprintf("%s: '%s' is not a valid UUID", apiv0.CommitsQueryLabel, raw), r.URL.Path))
		return uuid.Nil, false
	}
	return labelID, true
}
