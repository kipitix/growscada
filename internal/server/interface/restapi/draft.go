package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
)

// DraftHandlers serves the state of a Project's Draft.
type DraftHandlers struct {
	service application.DraftService
}

// NewDraftHandler creates the handler of the Draft's state.
func NewDraftHandler(s application.DraftService) *DraftHandlers {
	return &DraftHandlers{service: s}
}

// GetDraft handles GET /projects/{project_id}/draft: the Version of the
// Draft the project's content is versioned by (ADR 0016).
func (h DraftHandlers) GetDraft(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	state, err := h.service.State(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, apiv0.DraftStateResponse{Version: state.Version})
}
