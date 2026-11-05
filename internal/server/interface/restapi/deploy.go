package restapi

import (
	"net/http"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// DeployHandlers handles the deployment of a Project: the Revision it runs,
// the snapshot of its Tag states, the preview and the act of Deploy, and
// Discard (task 35, part 5). The Revision and the Tag states are the Runtime
// node's; Deploy and Discard are the Engineering node's.
type DeployHandlers struct {
	deploy    application.DeployService
	discard   application.DiscardService
	revisions runtimeapp.RevisionService
	states    runtimeapp.TagStateService
}

// NewDeployHandler creates the handler over the Engineering node's Deploy and
// Discard services and the Runtime node's Revisions and Tag states.
func NewDeployHandler(deploy application.DeployService, discard application.DiscardService,
	revisions runtimeapp.RevisionService, states runtimeapp.TagStateService) *DeployHandlers {
	return &DeployHandlers{deploy: deploy, discard: discard, revisions: revisions, states: states}
}

// currentRevision returns the Revision the Project runs, answering 404 itself
// when it runs none. A failed read is answered as well, so the caller returns
// on ok == false.
func (h DeployHandlers) currentRevision(w http.ResponseWriter, r *http.Request, projectID uuid.UUID) (runtimeapp.Revision, bool) {
	revision, deployed, err := h.revisions.FindCurrent(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return runtimeapp.Revision{}, false
	}
	if !deployed {
		sendProblem(w, NewNotFound("Revision of project", projectID.String(), r.URL.Path))
		return runtimeapp.Revision{}, false
	}
	return revision, true
}

// GetRevision handles GET /projects/{project_id}/revision: the Revision the
// Project runs, with its ProjectFile. 404 until the Project's first Deploy.
func (h DeployHandlers) GetRevision(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	revision, ok := h.currentRevision(w, r, projectID)
	if !ok {
		return
	}
	sendJSONResponse(w, http.StatusOK, newRevisionResponse(revision))
}

// GetTagStates handles GET /projects/{project_id}/tag-states: the states of
// the Tags of the running Revision, by Tag ID — the snapshot Operation renders
// before tag_updated arrives. 404 until the Project's first Deploy.
func (h DeployHandlers) GetTagStates(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	revision, ok := h.currentRevision(w, r, projectID)
	if !ok {
		return
	}
	tagIDs := make([]uuid.UUID, len(revision.Content.Tags))
	for i, definition := range revision.Content.Tags {
		tagIDs[i] = definition.ID
	}
	found, err := h.states.FindTagStates(r.Context(), projectID, tagIDs)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetTagStatesResponse(found))
}

// GetDeployPreview handles GET /projects/{project_id}/deploy/preview: the
// Revision the Project runs, whether its Draft is that Revision unchanged, and
// the runtime Widgets a Deploy would drop.
func (h DeployHandlers) GetDeployPreview(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	preview, err := h.deploy.Preview(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDeployPreviewResponse(preview))
}

// PostDeploy handles POST /projects/{project_id}/deploy: it makes the Draft
// the new Revision of the Project. 201 with the new Revision; 409
// nothing_to_deploy when the Draft is the Revision the Project runs.
func (h DeployHandlers) PostDeploy(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	revision, err := h.deploy.Deploy(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newDeployResponse(revision))
}

// PostDiscard handles POST /projects/{project_id}/discard: it marks the Project
// as being reset to the Revision it runs and answers at once (202); the Draft
// is read-only until the Discard ends. 409 without a Revision.
func (h DeployHandlers) PostDiscard(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	state, err := h.discard.Discard(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, apiv0.DiscardProjectResponse{Version: state.Version})
}
