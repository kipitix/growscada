package restapi

import (
	"net/http"

	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// RevisionHandlers handles the Revisions of a Project: the list, one Revision
// by number and the Rollback that copies an earlier one into a new one
// (task 36). All three are the Runtime node's and read no Draft (ADR 0014).
type RevisionHandlers struct {
	revisions runtimeapp.RevisionService
}

// NewRevisionHandler creates the handlers over the Runtime node's Revisions.
func NewRevisionHandler(revisions runtimeapp.RevisionService) *RevisionHandlers {
	return &RevisionHandlers{revisions: revisions}
}

// GetRevisions handles GET /projects/{project_id}/revisions: the Revisions of
// the Project, newest first, without their documents (task 36). A Project
// without a Revision answers an empty list; a deleted one, 404.
func (h RevisionHandlers) GetRevisions(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	summaries, err := h.revisions.List(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newListRevisionsResponse(summaries))
}

// GetRevisionByNumber handles GET /projects/{project_id}/revisions/{revision_number}:
// one Revision with its ProjectFile, as GET …/revision serves the running one.
func (h RevisionHandlers) GetRevisionByNumber(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, revisionResource)
	if !ok {
		return
	}
	revision, err := h.revisions.Find(r.Context(), projectID, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newRevisionResponse(revision))
}

// PostRollback handles POST /projects/{project_id}/revisions/{revision_number}/rollback:
// the Revision's document becomes a new Revision, without the Draft. 201 with
// the new Revision's summary; 404 for a number the Project does not have; 409
// nothing_to_rollback when its document is the running one, and
// project_discarding while a Discard runs.
func (h RevisionHandlers) PostRollback(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, revisionResource)
	if !ok {
		return
	}
	revision, err := h.revisions.Rollback(r.Context(), projectID, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newRollbackResponse(revision))
}
