package restapi_test

import (
	"context"
	"net/http"
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestDeploy_DeploymentPreviewAndTagStates(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()

	if rec := serve(router, http.MethodGet, mainPath+"/deployment", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET deployment before Deploy: %d, want 404", rec.Code)
	}
	if rec := serve(router, http.MethodGet, mainPath+"/tag-states", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("GET tag-states before Deploy: %d, want 404", rec.Code)
	}

	rec := serve(router, http.MethodGet, mainPath+"/deploy/preview", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET preview before Deploy: %d %s", rec.Code, rec.Body)
	}
	preview := decodeBody[apiv0.DeployPreviewResponse](t, rec)
	if preview.CurrentDeployment != nil || preview.Unchanged || preview.DroppedWidgets == nil || len(preview.DroppedWidgets) != 0 {
		t.Errorf("preview before Deploy: %+v", preview)
	}

	rec = serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Name: "speed", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	createdTag := decodeBody[apiv0.CreateTagResponse](t, rec)

	rec = serve(router, http.MethodPost, mainPath+"/deploy", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}
	deployed := decodeBody[apiv0.DeployResponse](t, rec)
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

	rec = serve(router, http.MethodGet, mainPath+"/deploy/preview", nil)
	preview = decodeBody[apiv0.DeployPreviewResponse](t, rec)
	if preview.CurrentDeployment == nil || preview.CurrentDeployment.Number != 1 || !preview.Unchanged {
		t.Errorf("preview after Deploy: %+v", preview)
	}

	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusConflict {
		t.Errorf("a second Deploy of the same Draft: %d, want 409", rec.Code)
	}
}

// TestDeployments_ListDetailRollback drives the three routes of task 36: the
// list (an empty array before the first Deploy), the detail with its document,
// and a Rollback that appends a copy of an earlier Deployment — with the
// refusals a bad number, a missing one and the running document get.
func TestDeployments_ListDetailRollback(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()

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

	rec = serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Name: "speed", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST tag: %d %s", rec.Code, rec.Body)
	}
	first := decodeBody[apiv0.CreateTagResponse](t, rec)
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}

	rec = serve(router, http.MethodGet, mainPath+"/deployments", nil)
	list := decodeBody[apiv0.ListDeploymentsResponse](t, rec)
	if len(list.Deployments) != 1 || list.Deployments[0].Number != 1 || list.Deployments[0].Cause != apiv0.DeploymentCauseDeploy {
		t.Fatalf("the list after the first Deploy: %+v", list.Deployments)
	}
	if list.Deployments[0].RolledBackFrom != nil {
		t.Errorf("a Deploy names a Deployment it copied: %v", list.Deployments[0].RolledBackFrom)
	}

	if rec := serve(router, http.MethodGet, mainPath+"/deployments/abc", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("GET a deployment number that is not a number: %d, want 400", rec.Code)
	}

	// The second Deployment differs from the first: another Tag.
	rec = serve(router, http.MethodPost, mainPath+"/tags", apiv0.CreateTagRequest{Name: "flow", Type: "integer"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST the second tag: %d %s", rec.Code, rec.Body)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("the second deploy: %d %s", rec.Code, rec.Body)
	}

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
	router := newServices().router()
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}
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

func TestDeploy_Discard(t *testing.T) {
	cleanProjectsRest(t)
	router := newServices().router()

	if rec := serve(router, http.MethodPost, mainPath+"/discard", nil); rec.Code != http.StatusConflict {
		t.Fatalf("POST discard without a Deployment: %d, want 409", rec.Code)
	}
	if rec := serve(router, http.MethodPost, mainPath+"/deploy", nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST deploy: %d %s", rec.Code, rec.Body)
	}

	rec := serve(router, http.MethodPost, mainPath+"/discard", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST discard: %d %s", rec.Code, rec.Body)
	}
	discarded := decodeBody[apiv0.DiscardProjectResponse](t, rec)
	if discarded.Version == 0 {
		t.Errorf("POST discard states no draft version: %+v", discarded)
	}
}
