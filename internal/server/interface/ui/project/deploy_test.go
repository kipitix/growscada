package project

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// deployDraft saves the Draft as a Commit, publishes it and deploys the
// Release: a Deploy takes a Release since task 72_4.
func deployDraft(t *testing.T, api *testAPI) apiv0.DeployResponse {
	t.Helper()
	ctx := context.Background()
	state, err := api.DraftState(ctx)
	if err != nil {
		t.Fatalf("draft state: %v", err)
	}
	commit, err := api.SaveCommit(ctx, apiv0.CreateCommitRequest{Version: state.Version, Title: "Save", Author: "Ada"})
	if err != nil {
		t.Fatalf("save commit: %v", err)
	}
	released, err := api.CreateRelease(ctx, apiv0.CreateProjectReleaseRequest{Commit: commit.Number, Author: "Ada"})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	deployed, err := api.DeployRelease(ctx, released.Number)
	if err != nil {
		t.Fatalf("deploy release: %v", err)
	}
	return deployed
}

// TestDeployBar_ShowsTheDeploymentAndItsRelease: the bar starts at "Not
// deployed", and after a Deploy it names the Deployment and the Release it
// came from.
func TestDeployBar_ShowsTheDeploymentAndItsRelease(t *testing.T) {
	f := startProject(t)

	if !f.p.deploymentLoaded || f.p.deployed {
		t.Fatalf("deployment state: loaded %v, deployed %v", f.p.deploymentLoaded, f.p.deployed)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Not deployed") {
		t.Errorf("the bar does not say the project is not deployed: %s", html)
	}

	deployDraft(t, f.api)
	f.p.loadDeploy(f.p.compoCtx)
	f.run()
	f.noToasts()
	if !f.p.deployed || f.p.deployment.Number != 1 || f.p.deployment.ReleaseNumber == nil || *f.p.deployment.ReleaseNumber != 1 {
		t.Fatalf("after Deploy: deployed %v, %+v", f.p.deployed, f.p.deployment)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Deployment 1") || !strings.Contains(html, "Release 1") {
		t.Errorf("the bar does not show Deployment 1 of Release 1: %s", html)
	}
}

// TestReleasesDialog_DeploysAReleaseAndMovesTheMark: the list marks the running
// Release and offers every other one, and a Deploy moves the mark (task 72_4).
func TestReleasesDialog_DeploysAReleaseAndMovesTheMark(t *testing.T) {
	f := startProject(t)
	deployDraft(t, f.api)
	// A second Commit and Release, not deployed yet.
	createScene(t, f.api, "Second")
	state, err := f.api.DraftState(context.Background())
	if err != nil {
		t.Fatalf("draft state: %v", err)
	}
	if _, err := f.api.SaveCommit(context.Background(), apiv0.CreateCommitRequest{
		Version: state.Version, Title: "Second save", Author: "Ada"}); err != nil {
		t.Fatalf("save commit: %v", err)
	}
	if _, err := f.api.CreateRelease(context.Background(), apiv0.CreateProjectReleaseRequest{Commit: 2, Author: "Ada"}); err != nil {
		t.Fatalf("release: %v", err)
	}

	f.p.openReleasesDialog(f.p.compoCtx)
	f.run()
	f.p.loadDeploy(f.p.compoCtx)
	f.run()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-release-running="1"`) {
		t.Errorf("Release 1 is not marked running: %s", html)
	}
	if !strings.Contains(html, `data-release-deploy="2"`) {
		t.Errorf("Release 2 is not offered for Deploy: %s", html)
	}

	f.p.doDeployRelease(f.p.compoCtx, 2)
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Release 2 deployed") {
		t.Errorf("the Deploy showed %+v, want its confirmation", got)
	}
	if f.p.deployment.ReleaseNumber == nil || *f.p.deployment.ReleaseNumber != 2 {
		t.Fatalf("the running Release after the Deploy: %+v", f.p.deployment)
	}
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-release-running="2"`) || strings.Contains(html, `data-release-running="1"`) {
		t.Errorf("the running mark did not move to Release 2: %s", html)
	}
}

// TestReleasesDialog_DeploymentWithoutRelease: a Deployment from a file, or one
// made before Releases existed, is shown as "not a Release" and a Release is
// offered (task 72_4).
func TestReleasesDialog_DeploymentWithoutRelease(t *testing.T) {
	f := startProject(t)
	f.p.deployed = true
	f.p.deploymentLoaded = true
	f.p.deployment = apiv0.DeploymentResponse{Number: 4, DeployedAt: "2026-11-04T10:00:00Z"}
	f.p.releases = []apiv0.ReleaseResponse{{Number: 1, Title: "First"}}
	f.p.releasesLoaded = true
	f.p.releasesDialog = true

	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Running Deployment 4 (not a Release)") {
		t.Errorf("the list does not say the Deployment is not a Release: %s", html)
	}
	if !strings.Contains(html, `data-release-deploy="1"`) {
		t.Errorf("the list does not offer the Release for Deploy: %s", html)
	}
}

// TestDeployRelease_ServerRefusalIsAToast: another user deploys the Release the
// list shows as deployable. The server's 409 must reach the user as a toast.
func TestDeployRelease_ServerRefusalIsAToast(t *testing.T) {
	f := startProject(t)
	deployDraft(t, f.api)
	f.p.loadDeploy(f.p.compoCtx)
	f.p.loadReleases(f.p.compoCtx)
	f.run()

	f.p.doDeployRelease(f.p.compoCtx, 1)
	f.run()

	if got := f.oneToast(); got.Status != http.StatusConflict || !strings.Contains(got.Detail, "Nothing to deploy") {
		t.Errorf("the refusal is not a toast: %+v", got)
	}
}

// failDeploymentRead fails the GET of the Deployment alone: the rest of the
// mode works, so a test sees what the Release list says when the running one
// cannot be checked.
type failDeploymentRead struct{}

func (failDeploymentRead) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/deployment") {
		return nil, errors.New("the deployment cannot be read")
	}
	return http.DefaultTransport.RoundTrip(r)
}

// TestReleasesDialog_UnknownDeployment: a failed read of the Deployment is not
// "not deployed": the list says the running Release cannot be checked, and the
// list itself stays readable because it is the Engineering node's (task 72_4).
func TestReleasesDialog_UnknownDeployment(t *testing.T) {
	url, api := newServer(t)
	deployDraft(t, api)
	f := mountWith(t, url, api, &http.Client{Transport: failDeploymentRead{}})
	f.run()
	f.e.TakeToasts() // the failed read of the Deployment is the toast of its load

	if !f.p.deploymentFailed || f.p.deploymentLoaded {
		t.Fatalf("the failed read: failed %v, loaded %v", f.p.deploymentFailed, f.p.deploymentLoaded)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Deployment unknown") {
		t.Errorf("the bar passes an unknown Deployment off as not deployed: %s", html)
	}

	f.p.openReleasesDialog(f.p.compoCtx)
	f.run()
	f.e.TakeToasts()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "data-release-running-unknown") {
		t.Errorf("the list does not say the running Release cannot be checked: %s", html)
	}
	if !strings.Contains(html, `data-release-row="1"`) {
		t.Errorf("the list is not shown without the Deployment: %s", html)
	}
}
