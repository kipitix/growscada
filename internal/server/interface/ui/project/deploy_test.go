package project

import (
	"context"
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
