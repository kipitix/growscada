package project

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestDeployBar_ShowsTheRevisionAndDeploys: the bar starts at "Not deployed",
// the dialog previews the first Deploy, and confirming makes Revision 1.
func TestDeployBar_ShowsTheRevisionAndDeploys(t *testing.T) {
	f := startProject(t)

	if !f.p.deployLoaded || f.p.deployPreview.CurrentRevision != nil {
		t.Fatalf("deployment state: loaded %v, %+v", f.p.deployLoaded, f.p.deployPreview)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Not deployed") {
		t.Errorf("the bar does not say the project is not deployed: %s", html)
	}

	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Deploy the project") || !strings.Contains(html, "not been deployed yet") {
		t.Errorf("the dialog does not preview the first Deploy: %s", html)
	}

	// A reload of the bar that lands while the dialog is open must not rewrite
	// it: the dialog keeps the preview it was opened with.
	f.p.deployPreview = apiv0.DeployPreviewResponse{
		CurrentRevision: &apiv0.RevisionSummaryResponse{Number: 7, DeployedAt: "2026-11-04T10:00:00Z"},
		Unchanged:       true,
	}
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Revision 7") || !strings.Contains(html, "not been deployed yet") {
		t.Errorf("the bar's reload rewrote the open dialog: %s", html)
	}

	f.p.deploy(f.p.compoCtx)
	f.run()
	f.noToasts()
	if f.p.deployPreview.CurrentRevision == nil || f.p.deployPreview.CurrentRevision.Number != 1 {
		t.Fatalf("after Deploy: %+v", f.p.deployPreview)
	}
	if f.p.deployDialog {
		t.Error("the dialog is still open after Deploy")
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Revision 1") {
		t.Errorf("the bar does not show Revision 1: %s", html)
	}
}

// TestDeployDialog_UnchangedDraftSaysNothingToDeploy: deploying the Revision
// the Draft is refuses, and the dialog says so before the server does.
func TestDeployDialog_UnchangedDraftSaysNothingToDeploy(t *testing.T) {
	url, api := newServer(t)
	if _, err := api.Deploy(context.Background()); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	f := mount(t, url, api)

	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	if !f.p.deployPreview.Unchanged {
		t.Fatalf("the deployed Draft is not unchanged: %+v", f.p.deployPreview)
	}
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Nothing to deploy") {
		t.Errorf("the dialog does not say nothing is to be deployed: %s", html)
	}
	if strings.Contains(html, `data-deploy-confirm="true"`) {
		t.Errorf("the dialog offers to Deploy an unchanged Draft: %s", html)
	}
}

// TestDeployBar_DraftChangeShowsUnreleasedChanges: a content event reloads the
// deployment state, and a changed Draft is shown as not deployed.
func TestDeployBar_DraftChangeShowsUnreleasedChanges(t *testing.T) {
	url, api := newServer(t)
	if _, err := api.Deploy(context.Background()); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	f := mount(t, url, api)
	if f.p.deployPreview.CurrentRevision == nil || !f.p.deployPreview.Unchanged {
		t.Fatalf("after Deploy: %+v", f.p.deployPreview)
	}

	created := createScene(t, api, "After the Deploy")
	f.event(apiv0.Event{Type: apiv0.EventSceneCreated, ID: created.String()})

	f.noToasts()
	if f.p.deployPreview.Unchanged {
		t.Errorf("the Draft changed, but the bar still says it is deployed: %+v", f.p.deployPreview)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Unreleased changes") {
		t.Errorf("the bar does not show the unreleased changes: %s", html)
	}
}

// TestDeploy_ServerRefusalIsAToast: the dialog was previewed against a changed
// Draft, and another user deployed it before the confirmation. The server's
// 409 must reach the user as a toast, not as a silent failure.
func TestDeploy_ServerRefusalIsAToast(t *testing.T) {
	url, api := newServer(t)
	f := mount(t, url, api)

	f.p.openDeployDialog(f.p.compoCtx)
	f.run()
	if f.p.dialogPreview.Unchanged {
		t.Fatalf("the dialog previews an unchanged Draft: %+v", f.p.dialogPreview)
	}

	if _, err := api.Deploy(context.Background()); err != nil {
		t.Fatalf("deploy from another user: %v", err)
	}
	f.p.deploy(f.p.compoCtx)
	f.run()

	if f.p.deployDialog {
		t.Error("the dialog is still open after the server refused the Deploy")
	}
	if got := f.oneToast(); got.Status != http.StatusConflict || !strings.Contains(got.Detail, "Nothing to deploy") {
		t.Errorf("the refusal is not a toast: %+v", got)
	}
}
