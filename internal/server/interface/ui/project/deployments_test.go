package project

import (
	"context"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// deployTwice deploys the Draft of Main with one Scene (Deployment 1), adds a
// second Scene and deploys again (Deployment 2): the history the list shows.
func deployTwice(t *testing.T, api *testAPI) {
	t.Helper()
	createScene(t, api, "First")
	deployDraft(t, api)
	createScene(t, api, "Second")
	deployDraft(t, api)
}

// TestDeploymentsDialog_EmptyBeforeTheFirstDeploy: the button is always there,
// and an undeployed project's dialog says so instead of failing.
func TestDeploymentsDialog_EmptyBeforeTheFirstDeploy(t *testing.T) {
	f := startProject(t)

	f.p.openDeploymentsDialog(f.p.compoCtx)
	f.run()
	f.noToasts()

	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-deployments-dialog`) {
		t.Fatalf("the Deployments dialog is not open: %s", html)
	}
	if !strings.Contains(html, "has not been deployed yet") {
		t.Errorf("the dialog of an undeployed project is not empty: %s", html)
	}
}

// TestDeploymentsDialog_ListsTheDeploymentsAndMarksTheRunningOne: the list is
// newest first, the running Deployment is marked, and it shows the Release the
// Deployment came from and its document as counts. Rollback is not offered by
// the Project mode (task 72_4).
func TestDeploymentsDialog_ListsTheDeploymentsAndMarksTheRunningOne(t *testing.T) {
	url, api := newServer(t)
	deployTwice(t, api)
	f := mount(t, url, api)

	f.p.openDeploymentsDialog(f.p.compoCtx)
	f.run()
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	for _, want := range []string{`data-deployment-row="1"`, `data-deployment-row="2"`, "Deployed", `data-deployment-current`, "Release 2"} {
		if !strings.Contains(html, want) {
			t.Errorf("the list does not show %s: %s", want, html)
		}
	}

	f.p.selectDeployment(f.p.compoCtx, 1)
	f.run()
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-deployment-summary`) {
		t.Fatalf("the selected Deployment has no summary: %s", html)
	}
	if !strings.Contains(html, "1 scenes, 0 widgets") {
		t.Errorf("the summary does not count the document: %s", html)
	}
	if strings.Contains(html, `data-deployment-rollback`) {
		t.Errorf("the Project mode offers a Rollback: %s", html)
	}
}

// TestDeploymentsDialog_ReloadsOnDeploymentCreated: another user's Deploy while
// the dialog is open shows up in it.
func TestDeploymentsDialog_ReloadsOnDeploymentCreated(t *testing.T) {
	url, api := newServer(t)
	createScene(t, api, "First")
	deployDraft(t, api)
	f := mount(t, url, api)
	f.p.openDeploymentsDialog(f.p.compoCtx)
	f.run()

	createScene(t, api, "After the Deploy")
	deployDraft(t, api)
	f.event(apiv0.Event{Type: apiv0.EventDeploymentCreated})
	f.noToasts()

	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-deployment-row="2"`) {
		t.Errorf("the open list did not pick up the new Deployment: %s", html)
	}
}

// TestDeploymentCreatedEventReloadsTheReleaseList: a Deploy in another tab
// moves the running mark without a reload of the page (task 72_4).
func TestDeploymentCreatedEventReloadsTheReleaseList(t *testing.T) {
	url, api := newServer(t)
	createScene(t, api, "First")
	deployDraft(t, api)
	createScene(t, api, "Second")
	state, err := api.DraftState(context.Background())
	if err != nil {
		t.Fatalf("draft state: %v", err)
	}
	if _, err := api.SaveCommit(context.Background(), apiv0.CreateCommitRequest{Version: state.Version, Title: "Second", Author: "Ada"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := api.CreateRelease(context.Background(), apiv0.CreateProjectReleaseRequest{Commit: 2, Author: "Ada"}); err != nil {
		t.Fatalf("release: %v", err)
	}
	f := mount(t, url, api)
	f.p.releasesDialog = true
	f.p.loadReleases(f.p.compoCtx)
	f.p.loadDeploy(f.p.compoCtx)
	f.run()

	if _, err := api.DeployRelease(context.Background(), 2); err != nil {
		t.Fatalf("deploy release: %v", err)
	}
	f.event(apiv0.Event{Type: apiv0.EventDeploymentCreated})
	f.run()
	f.noToasts()

	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-release-running="2"`) {
		t.Errorf("the release list did not follow the Deployment: %s", html)
	}
}
