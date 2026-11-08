package restapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application"
	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// DeployHandlers handles the deployment of a Project: the Deployment it runs,
// the snapshot of its Tag states, the preview and the act of Deploy, and
// Discard (task 35, part 5). The Deployment and the Tag states are the Runtime
// node's; Deploy and Discard are the Engineering node's.
type DeployHandlers struct {
	deploy      application.DeployService
	discard     application.DiscardService
	deployments runtimeapp.DeploymentService
	states      runtimeapp.TagStateService
}

// NewDeployHandler creates the handler over the Engineering node's Deploy and
// Discard services and the Runtime node's Deployments and Tag states.
func NewDeployHandler(deploy application.DeployService, discard application.DiscardService,
	deployments runtimeapp.DeploymentService, states runtimeapp.TagStateService) *DeployHandlers {
	return &DeployHandlers{deploy: deploy, discard: discard, deployments: deployments, states: states}
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

// GetDeployPreview handles GET /projects/{project_id}/deploy/preview: the
// Deployment the Project runs, whether its Draft is that Deployment unchanged, and
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
// the new Deployment of the Project. 201 with the new Deployment; 409
// nothing_to_deploy when the Draft is the Deployment the Project runs.
func (h DeployHandlers) PostDeploy(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	deployment, err := h.deploy.Deploy(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newDeployResponse(deployment))
}

// PostDiscard handles POST /projects/{project_id}/discard: it resets the Draft
// to the Deployment the Project runs and answers the Draft's new state, with
// the Library Releases it could not restore (task 71). 409 without a
// Deployment.
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
