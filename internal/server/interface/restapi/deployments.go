package restapi

import (
	"net/http"

	runtimeapp "github.com/kipitix/growscada/internal/server/application/runtime"
)

// DeploymentHandlers handles the Deployments of a Project: the list, one Deployment
// by number and the Rollback that copies an earlier one into a new one
// (task 36). All three are the Runtime node's and read no Draft (ADR 0014).
type DeploymentHandlers struct {
	deployments runtimeapp.DeploymentService
}

// NewDeploymentHandler creates the handlers over the Runtime node's Deployments.
func NewDeploymentHandler(deployments runtimeapp.DeploymentService) *DeploymentHandlers {
	return &DeploymentHandlers{deployments: deployments}
}

// GetDeployments handles GET /projects/{project_id}/deployments: the Deployments of
// the Project, newest first, without their documents (task 36). A Project
// without a Deployment answers an empty list; a deleted one, 404.
func (h DeploymentHandlers) GetDeployments(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	summaries, err := h.deployments.List(r.Context(), projectID)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newListDeploymentsResponse(summaries))
}

// GetDeploymentByNumber handles GET /projects/{project_id}/deployments/{deployment_number}:
// one Deployment with its ProjectFile, as GET …/deployment serves the running one.
func (h DeploymentHandlers) GetDeploymentByNumber(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, deploymentResource)
	if !ok {
		return
	}
	deployment, err := h.deployments.Find(r.Context(), projectID, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusOK, newDeploymentResponse(deployment))
}

// PostRollback handles POST /projects/{project_id}/deployments/{deployment_number}/rollback:
// the Deployment's document becomes a new Deployment, without the Draft. 201 with
// the new Deployment's summary; 404 for a number the Project does not have; 409
// nothing_to_rollback when its document is the running one.
func (h DeploymentHandlers) PostRollback(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, projectResource)
	if !ok {
		return
	}
	number, ok := pathInt(w, r, deploymentResource)
	if !ok {
		return
	}
	deployment, err := h.deployments.Rollback(r.Context(), projectID, number)
	if err != nil {
		sendError(w, r, err)
		return
	}
	sendJSONResponse(w, http.StatusCreated, newRollbackResponse(deployment))
}
