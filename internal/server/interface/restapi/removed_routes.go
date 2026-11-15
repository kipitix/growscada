package restapi

import (
	"net/http"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// removedDraftLockPath is where a Draft's lock lived before ADR 0022: a
// Project's and a Library's. The routes stay, answering 410 Gone, so an old
// tab or tool learns the lock is gone instead of getting a bare 404; they are
// removed when the API reaches 1.0, as the pre-versioning path is.
const removedDraftLockPath = "/edit-lock"

// draftLockRemoved answers every removed lock route: the Draft is edited
// and saved freely, and its Version is the only concurrency token (ADR 0022).
func draftLockRemoved(w http.ResponseWriter, r *http.Request) {
	sendProblem(w, NewGone(
		"The editing lock is gone: edit and save the Draft; the Version of the Draft is the only concurrency token",
		r.URL.Path))
}

// removedDeploymentPath and removedDeploymentsPath are where a Deployment lived
// before task 72_9: the routes told an old reader the Revision is gone and
// named the Deployment routes instead. They are removed when the API reaches
// 1.0, as the pre-versioning path is.
const (
	removedDeploymentPath  = "/revision"
	removedDeploymentsPath = "/revisions"
)

// deploymentRenamed answers every removed Deployment route: the runtime entity
// is a Deployment now, and its routes name it.
func deploymentRenamed(w http.ResponseWriter, r *http.Request) {
	sendProblem(w, NewGone(
		"Revision is called Deployment: use "+apiv0.PathPrefix+"/projects/{project_id}/deployment and …/deployments",
		r.URL.Path))
}

// removedSavedStatesPath is where the saved states of a Draft lived before task
// 88_2: the term Commit went, and the routes name saved states. They stay,
// answering 410 Gone, so an old tool learns the new path instead of getting a
// bare 404; they are removed when the API reaches 1.0, as the pre-versioning
// path is (ADR 0026).
const removedSavedStatesPath = "/commits"

// savedStatesRenamed answers every removed saved state route: a Commit is a
// saved state now, and its routes name it.
func savedStatesRenamed(w http.ResponseWriter, r *http.Request) {
	sendProblem(w, NewGone(
		"Commit is called saved state: use "+apiv0.PathPrefix+
			"/projects/{project_id}/saved-states and "+apiv0.PathPrefix+"/libraries/{library_id}/saved-states",
		r.URL.Path))
}
