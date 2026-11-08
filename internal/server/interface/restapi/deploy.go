package restapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// DeployHandlers handles the deployment of a Project: the Deployment it runs,
// the snapshot of its Tag states, Discard and Revert (task 35, part 5; task
// 72_4). The Deployment and the Tag states are the Runtime node's; Discard and
// Revert are the Engineering node's. A Deploy of a Release is served by the
// Release handlers, and the old Deploy of the Draft answers 410.
type DeployHandlers struct {
	discard     application.DiscardService
	deployments runtimeapp.DeploymentService
	states      runtimeapp.TagStateService
}

// NewDeployHandler creates the handler over the Engineering node's Discard and
// Revert services and the Runtime node's Deployments and Tag states.
func NewDeployHandler(discard application.DiscardService,
	deployments runtimeapp.DeploymentService, states runtimeapp.TagStateService) *DeployHandlers {
	return &DeployHandlers{discard: discard, deployments: deployments, states: states}
}

// currentDeployment returns the Deployment the Project runs, answering 404 itself
// when it runs none. A failed read is answered as well, so the caller returns
// on ok == false.
func (h DeployHandlers) currentDeployment(w http.ResponseWriter, r *http.Request, projectID uuid.UUID) (runtimeapp.Deployment, bool) {
	deployment, deployed, err := h.deployments.FindCurrent(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return runtimeapp.Deployment{}, false
	}
	if !deployed {
		sendProblem(w, NewNotFound("Deployment of project", projectID.String(), r.URL.Path))
		return runtimeapp.Deployment{}, false
	}
	return deployment, true
}

// GetDeployment handles GET /projects/{project_id}/deployment: the Deployment the
// Project runs, with its ProjectFile. 404 until the Project's first Deploy.
func (h DeployHandlers) GetDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	deployment, ok := h.currentDeployment(w, r, projectID)
	if !ok {
		return
	}
	sendJSONResponse(w, http.StatusOK, newDeploymentResponse(deployment))
}

// GetTagStates handles GET /projects/{project_id}/tag-states: the states of
// the Tags of the running Deployment, by Tag ID — the snapshot Operation renders
// before tag_updated arrives. 404 until the Project's first Deploy.
func (h DeployHandlers) GetTagStates(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	deployment, ok := h.currentDeployment(w, r, projectID)
	if !ok {
		return
	}
	tagIDs := make([]uuid.UUID, len(deployment.Content.Tags))
	for i, definition := range deployment.Content.Tags {
		tagIDs[i] = definition.ID
	}
	found, err := h.states.FindTagStates(r.Context(), projectID, tagIDs)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newGetTagStatesResponse(found))
}

// PostDiscard handles POST /projects/{project_id}/discard: it resets the Draft
// to the Project's last Commit, dropping the unsaved changes, and answers the
// Draft's new state, with the Library Releases it could not restore (task 71,
// task 72_4). 409 nothing_to_discard without a Commit.
func (h DeployHandlers) PostDiscard(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	discarded, err := h.discard.Discard(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDiscardProjectResponse(discarded))
}

// PostRevert handles POST /projects/{project_id}/revert-to-deployed: it resets
// the Draft to the Deployment the Project runs — the Discard of before task
// 72_4 — and answers the Draft's new state. 409 without a Deployment.
func (h DeployHandlers) PostRevert(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	reverted, err := h.discard.Revert(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDiscardProjectResponse(reverted))
}

// deployDraftRemoved answers POST /projects/{project_id}/deploy: a Deploy took
// the Draft before task 72_4 and takes a Release now. The route stays, answering
// 410, so an old tab or tool learns the order instead of getting a bare 404;
// it is removed when the API reaches 1.0.
func deployDraftRemoved(w http.ResponseWriter, r *http.Request) {
	sendProblem(w, NewGone(
		"A Deploy takes a Release: Save → Release → Deploy (POST "+"/projects/{project_id}/releases/{release_number}/deploy)",
		r.URL.Path))
}
