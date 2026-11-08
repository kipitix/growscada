package restapi_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// saveReleaseDeploy fixes the Draft as a Commit, publishes it and makes it the
// running Deployment: the order a Deploy takes since task 72_4.
func saveReleaseDeploy(t *testing.T, router testRouter, version, commitNumber int) apiv0.DeployResponse {
	t.Helper()
	rec := serve(router, http.MethodPost, mainPath+"/commits",
		apiv0.CreateCommitRequest{Version: version, Title: "Save", Author: "Ada"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST commit: %d %s", rec.Code, rec.Body)
	}
	rec = serve(router, http.MethodPost, mainPath+"/releases",
		apiv0.CreateProjectReleaseRequest{Commit: commitNumber, Author: "Ada"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST release: %d %s", rec.Code, rec.Body)
	}
	rec = serve(router, http.MethodPost, mainPath+"/releases/"+strconv.Itoa(commitNumber)+"/deploy", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST release deploy: %d %s", rec.Code, rec.Body)
	}
	return decodeBody[apiv0.DeployResponse](t, rec)
}

func TestDeployRelease_DeploymentAndTagStates(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	router := services.router()
	version := services.draftVersion(t)

	if rec := serve(router, http.MethodGet, mainPath+"/deployment", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET deployment before Deploy: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/tag-states", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET tag-states before Deploy: %d, want 404", rec.Code)
	}

	rec := serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Version: version, Name: "speed", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	createdTag := decodeBody[apiv0.CreateTagResponse](t, rec)
	version = services.draftVersion(t)

	deployed := saveReleaseDeploy(t, router, version, 1)
	if deployed.Number != 1 || deployed.DeployedAt == "" {
		t.Errorf("the first Deploy: %+v", deployed)
	}

	rec = serve(router, http.MethodGet, mainPath+"/deployment", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET deployment: %d %s", rec.Code, rec.Body)
	}
	deployment := decodeBody[apiv0.DeploymentResponse](t, rec)
	if deployment.Number != 1 || deployment.DeployedAt == "" || deployment.Content.ProjectID != mainProject {
		t.Errorf("GET deployment: %+v", deployment)
	}
	if deployment.ReleaseNumber == nil || *deployment.ReleaseNumber != 1 {
		t.Errorf("the Deployment names release %v, want 1", deployment.ReleaseNumber)
	}
	if len(deployment.Content.Tags) == 0 {
		t.Errorf("the Deployment holds no Tag definitions: %+v", deployment.Content)
	}

	rec = serve(router, http.MethodGet, mainPath+"/tag-states", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET tag-states: %d %s", rec.Code, rec.Body)
	}
	states := decodeBody[apiv0.GetTagStatesResponse](t, rec)
	if len(states.TagStates) == 0 {
		t.Errorf("the snapshot holds no Tag states: %+v", states)
	}
	if state, ok := states.TagStates[createdTag.ID.String()]; !ok || state.Quality != "bad" || state.Seq != 0 {
		t.Errorf("the initial state of the new Tag: %+v, %v", state, ok)
	}

	if rec := serve(router, http.MethodPost, mainPath+"/releases/1/deploy", nil); rec.Code != http.StatusConflict {
		t.Errorf("a second Deploy of the same Release: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/releases/9/deploy", nil); rec.Code != http.StatusNotFound {
		t.Errorf("a Deploy of a missing Release: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusGone {
		t.Errorf("the removed Deploy of the Draft: %d, want 410", rec.Code)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/deploy/preview", nil); rec.Code != http.StatusNotFound {
		t.Errorf("the removed Deploy preview: %d, want 404", rec.Code)
	}
}

// TestDeployments_ListDetailRollback drives the three routes of task 36: the
// list (an empty array before the first Deploy), the detail with its document,
// and a Rollback that appends a copy of an earlier Deployment — with the
// refusals a bad number, a missing one and the running document get.
func TestDeployments_ListDetailRollback(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	router := services.router()

	rec := serve(router, http.MethodGet, mainPath+"/deployments", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET deployments before the Deploy: %d %s", rec.Code, rec.Body)
	}
	if list := decodeBody[apiv0.ListDeploymentsResponse](t, rec); list.Deployments == nil || len(list.Deployments) != 0 {
		t.Errorf("the list of a project without a Deployment: %+v", list.Deployments)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/deployments/1", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a Deployment of a project without one: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/deployments/1/rollback", nil); rec.Code != http.StatusNotFound {
		t.Errorf("Rollback of a project without a Deployment: %d, want 404", rec.Code)
	}

	rec = serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Version: services.draftVersion(t), Name: "speed", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	first := decodeBody[apiv0.CreateTagResponse](t, rec)
	saveReleaseDeploy(t, router, services.draftVersion(t), 1)

	rec = serve(router, http.MethodGet, mainPath+"/deployments", nil)
	list := decodeBody[apiv0.ListDeploymentsResponse](t, rec)
	if len(list.Deployments) != 1 || list.Deployments[0].Number != 1 || list.Deployments[0].Cause != apiv0.DeploymentCauseDeploy {
		t.Fatalf("the list after the first Deploy: %+v", list.Deployments)
	}
	if list.Deployments[0].RolledBackFrom != nil {
		t.Errorf("a Deploy names a Deployment it copied: %v", list.Deployments[0].RolledBackFrom)
	}
	if list.Deployments[0].ReleaseNumber == nil || *list.Deployments[0].ReleaseNumber != 1 {
		t.Errorf("the Deployment names release %v, want 1", list.Deployments[0].ReleaseNumber)
	}

	if rec := serve(router, http.MethodGet, mainPath+"/deployments/abc", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("GET a deployment number that is not a number: %d, want 400", rec.Code)
	}

	// The second Deployment differs from the first: another Tag.
	rec = serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Version: services.draftVersion(t), Name: "flow", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST the second tag: %d %s", rec.Code, rec.Body)
	}
	saveReleaseDeploy(t, router, services.draftVersion(t), 2)

	rec = serve(router, http.MethodGet, mainPath+"/deployments/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET deployment 1: %d %s", rec.Code, rec.Body)
	}
	firstDeployment := decodeBody[apiv0.DeploymentResponse](t, rec)
	if firstDeployment.Number != 1 || firstDeployment.Cause != apiv0.DeploymentCauseDeploy {
		t.Errorf("deployment 1: %+v", firstDeployment)
	}
	if len(firstDeployment.Content.Tags) != 1 || firstDeployment.Content.Tags[0].ID != first.ID {
		t.Errorf("deployment 1 carries %+v, want only the first tag", firstDeployment.Content.Tags)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/deployments/99", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a missing deployment: %d, want 404", rec.Code)
	}

	// The running Deployment's own document is nothing to roll back to.
	if rec := serve(router, http.MethodPost, mainPath+"/deployments/2/rollback", nil); rec.Code != http.StatusConflict {
		t.Errorf("Rollback to the running deployment: %d, want 409", rec.Code)
	}

	rec = serve(router, http.MethodPost, mainPath+"/deployments/1/rollback", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Rollback to deployment 1: %d %s", rec.Code, rec.Body)
	}
	rolled := decodeBody[apiv0.DeploymentSummaryResponse](t, rec)
	if rolled.Number != 3 || rolled.Cause != apiv0.DeploymentCauseRollback {
		t.Errorf("the rolled-back deployment: %+v", rolled)
	}
	if rolled.RolledBackFrom == nil || *rolled.RolledBackFrom != 1 {
		t.Errorf("the rollback names %v, want deployment 1", rolled.RolledBackFrom)
	}
	if rolled.ReleaseNumber == nil || *rolled.ReleaseNumber != 1 {
		t.Errorf("the rollback names release %v, want 1", rolled.ReleaseNumber)
	}

	rec = serve(router, http.MethodGet, mainPath+"/deployment", nil)
	running := decodeBody[apiv0.DeploymentResponse](t, rec)
	if rec.Code != http.StatusOK || running.Number != 3 || running.Cause != apiv0.DeploymentCauseRollback {
		t.Fatalf("the running deployment after the Rollback: %d %+v", rec.Code, running)
	}
	if len(running.Content.Tags) != 1 || running.Content.Tags[0].ID != first.ID {
		t.Errorf("the running document is %+v, want the copy of deployment 1", running.Content.Tags)
	}

	rec = serve(router, http.MethodGet, mainPath+"/deployments", nil)
	list = decodeBody[apiv0.ListDeploymentsResponse](t, rec)
	if len(list.Deployments) != 3 || list.Deployments[0].Number != 3 || list.Deployments[0].Cause != apiv0.DeploymentCauseRollback {
		t.Errorf("the list after the Rollback: %+v", list.Deployments)
	}
}

// TestDeployments_OfADeletedProject_Are404: a deleted Project keeps its rows
// until its cleanup removes them, but they are not its Deployments to serve.
func TestDeployments_OfADeletedProject_Are404(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	router := services.router()
	saveReleaseDeploy(t, router, services.draftVersion(t), 1)
	if _, err := testDB().ExecContext(context.Background(),
		"UPDATE projects SET deleted_at = now() WHERE id = '"+mainProject.String()+"'"); err != nil {
		t.Fatalf("mark the project deleted: %v", err)
	}
	t.Cleanup(func() {
		if _, err := testDB().ExecContext(context.Background(),
			"UPDATE projects SET deleted_at = NULL WHERE id = '"+mainProject.String()+"'"); err != nil {
			t.Errorf("clear the deletion mark: %v", err)
		}
	})

	if rec := serve(router, http.MethodGet, mainPath+"/deployments", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET deployments of a deleted project: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/deployments/1", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET deployment 1 of a deleted project: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/deployments/1/rollback", nil); rec.Code != http.StatusNotFound {
		t.Errorf("Rollback of a deleted project: %d, want 404", rec.Code)
	}
}

func TestDiscardAndRevert(t *testing.T) {
	cleanProjectsRest(t)
	services := newServices()
	router := services.router()

	if rec := serve(router, http.MethodPost, mainPath+"/discard", nil); rec.Code != http.StatusConflict {
		t.Fatalf("POST discard without a Commit: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/revert-to-deployed", nil); rec.Code != http.StatusConflict {
		t.Fatalf("POST revert without a Deployment: %d, want 409", rec.Code)
	}
	saveReleaseDeploy(t, router, services.draftVersion(t), 1)

	rec := serve(router, http.MethodPost, mainPath+"/discard", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST discard: %d %s", rec.Code, rec.Body)
	}
	discarded := decodeBody[apiv0.DiscardProjectResponse](t, rec)
	if discarded.Version == 0 {
		t.Errorf("POST discard states no draft version: %+v", discarded)
	}
	rec = serve(router, http.MethodPost, mainPath+"/revert-to-deployed", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST revert: %d %s", rec.Code, rec.Body)
	}
	if reverted := decodeBody[apiv0.DiscardProjectResponse](t, rec); reverted.Version == 0 {
		t.Errorf("POST revert states no draft version: %+v", reverted)
	}
}
