package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
)

// DraftChangesHandlers serves the DraftChange stack of a Project's and a
// Library's Draft (ADR 0017): its state, its undo and its redo.
type DraftChangesHandlers struct {
	service application.DraftChangesService
}

// NewDraftChangesHandler creates the handler of the DraftChanges.
func NewDraftChangesHandler(s application.DraftChangesService) *DraftChangesHandlers {
	return &DraftChangesHandlers{service: s}
}

// GetProjectDraftChanges handles GET /projects/{project_id}/draft-changes.
func (h DraftChangesHandlers) GetProjectDraftChanges(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	h.get(w, r, draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID))))
}

// PostProjectDraftChangesUndo handles POST /projects/{project_id}/draft-changes/undo.
func (h DraftChangesHandlers) PostProjectDraftChangesUndo(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	h.command(w, r, draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID))), true)
}

// PostProjectDraftChangesRedo handles POST /projects/{project_id}/draft-changes/redo.
func (h DraftChangesHandlers) PostProjectDraftChangesRedo(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	h.command(w, r, draft.ProjectScope(id.NewID(id.IDWithUUID[project.Project](projectID))), false)
}

// GetLibraryDraftChanges handles GET /libraries/{library_id}/draft-changes.
func (h DraftChangesHandlers) GetLibraryDraftChanges(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	h.get(w, r, draft.LibraryScope(id.NewID(id.IDWithUUID[library.Library](libraryID))))
}

// PostLibraryDraftChangesUndo handles POST /libraries/{library_id}/draft-changes/undo.
func (h DraftChangesHandlers) PostLibraryDraftChangesUndo(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	h.command(w, r, draft.LibraryScope(id.NewID(id.IDWithUUID[library.Library](libraryID))), true)
}

// PostLibraryDraftChangesRedo handles POST /libraries/{library_id}/draft-changes/redo.
func (h DraftChangesHandlers) PostLibraryDraftChangesRedo(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathUUID(w, r, libraryResource)
	if !ok {
		return
	}
	h.command(w, r, draft.LibraryScope(id.NewID(id.IDWithUUID[library.Library](libraryID))), false)
}

// get answers the state of the stack of scope.
func (h DraftChangesHandlers) get(w http.ResponseWriter, r *http.Request, scope draft.Scope) {
	state, err := h.service.State(r.Context(), scope)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDraftChangesResponse(state))
}

// command applies an undo (undo true) or a redo of scope, stating the Version
// of the Draft the body names.
func (h DraftChangesHandlers) command(w http.ResponseWriter, r *http.Request, scope draft.Scope, undo bool) {
	var request apiv0.DraftChangeCommandRequest
	if !decodeRequest(w, r, &request) {
		return
	}
	var (
		state appdto.DraftChanges
		err   error
	)
	if undo {
		state, err = h.service.Undo(r.Context(), scope, request.Version)
	} else {
		state, err = h.service.Redo(r.Context(), scope, request.Version)
	}
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDraftChangesResponse(state))
}
