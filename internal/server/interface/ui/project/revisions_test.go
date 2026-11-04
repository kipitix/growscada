package project

import (
	"context"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// deployTwice deploys the Draft of Main with one Scene (Revision 1), adds a
// second Scene and deploys again (Revision 2): the history a Rollback can go
// back through.
func deployTwice(t *testing.T, api *testAPI) {
	t.Helper()
	ctx := context.Background()
	createScene(t, api, "First")
	if _, err := api.Deploy(ctx); err != nil {
		t.Fatalf("deploy 1: %v", err)
	}
	createScene(t, api, "Second")
	if _, err := api.Deploy(ctx); err != nil {
		t.Fatalf("deploy 2: %v", err)
	}
}

// TestRevisionsDialog_EmptyBeforeTheFirstDeploy: the button is always there,
// and an undeployed project's dialog says so instead of failing.
func TestRevisionsDialog_EmptyBeforeTheFirstDeploy(t *testing.T) {
	f := startProject(t)

	f.p.openRevisionsDialog(f.p.compoCtx)
	f.run()
	f.noToasts()

	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-revisions-dialog`) {
		t.Fatalf("the Revisions dialog is not open: %s", html)
	}
	if !strings.Contains(html, "has not been deployed yet") {
		t.Errorf("the dialog of an undeployed project is not empty: %s", html)
	}
}

// TestRevisionsDialog_ListsTheRevisionsAndMarksTheRunningOne: the list is
// newest first, the running Revision is marked, and its rollback is not
// offered; another one shows its document as counts.
func TestRevisionsDialog_ListsTheRevisionsAndMarksTheRunningOne(t *testing.T) {
	url, api := newServer(t)
	deployTwice(t, api)
	f := mount(t, url, api)

	f.p.openRevisionsDialog(f.p.compoCtx)
	f.run()
	f.noToasts()
	html := app.HTMLString(f.p.Render())
	for _, want := range []string{`data-revision-row="1"`, `data-revision-row="2"`, "Deployed", `data-revision-current`} {
		if !strings.Contains(html, want) {
			t.Errorf("the list does not show %s: %s", want, html)
		}
	}

	f.p.selectRevision(f.p.compoCtx, 1)
	f.run()
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-revision-summary`) {
		t.Fatalf("the selected Revision has no summary: %s", html)
	}
	if !strings.Contains(html, "1 scenes, 0 widgets") {
		t.Errorf("the summary does not count the document: %s", html)
	}
	if !strings.Contains(html, `data-revision-rollback`) {
		t.Errorf("the dialog does not offer the Rollback: %s", html)
	}

	f.p.selectRevision(f.p.compoCtx, 2)
	f.run()
	html = app.HTMLString(f.p.Render())
	if strings.Contains(html, `data-revision-rollback`) {
		t.Errorf("the dialog offers to roll back the running Revision: %s", html)
	}
}

// TestRollback_AppendsARevisionAndUpdatesTheBadge: a Rollback makes a new
// Revision with the old document, and the bar and the list say so.
func TestRollback_AppendsARevisionAndUpdatesTheBadge(t *testing.T) {
	url, api := newServer(t)
	deployTwice(t, api)
	f := mount(t, url, api)

	f.p.openRevisionsDialog(f.p.compoCtx)
	f.run()
	f.p.selectRevision(f.p.compoCtx, 1)
	f.run()
	f.p.doRollback(f.p.compoCtx, 1)
	f.run()
	f.noToasts()

	if f.p.revisionsDialog {
		t.Error("the Revisions dialog is still open after the Rollback")
	}
	if f.p.deployPreview.CurrentRevision == nil || f.p.deployPreview.CurrentRevision.Number != 3 {
		t.Fatalf("the bar's Revision after the Rollback: %+v", f.p.deployPreview.CurrentRevision)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, "Revision 3") {
		t.Errorf("the bar does not show Revision 3: %s", html)
	}

	f.p.openRevisionsDialog(f.p.compoCtx)
	f.run()
	f.p.selectRevision(f.p.compoCtx, 3)
	f.run()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Rolled back from #1") {
		t.Errorf("the list does not name what the Rollback copied: %s", html)
	}
	if !strings.Contains(html, `data-revision-row="3"`) || !strings.Contains(html, `data-revision-current`) {
		t.Errorf("the new Revision is not the running one in the list: %s", html)
	}
}

// TestRollback_ServerRefusalIsAToast: another user rolled the project back
// (or it was already refused) — the server's 409 must reach the user, and the
// dialog stay open.
func TestRollback_ServerRefusalIsAToast(t *testing.T) {
	url, api := newServer(t)
	deployTwice(t, api)
	f := mount(t, url, api)

	f.p.openRevisionsDialog(f.p.compoCtx)
	f.run()
	// The running Revision's own document: the current one changed between the
	// dialog's list and the click.
	f.p.doRollback(f.p.compoCtx, 2)
	f.run()

	if !f.p.revisionsDialog {
		t.Error("the dialog closed on the server's refusal")
	}
	if got := f.oneToast(); got.Status == 0 || !strings.Contains(got.Detail, "Nothing to rollback") {
		t.Errorf("the refusal is not a toast: %+v", got)
	}
}

// TestRevisionsDialog_ReloadsOnRevisionCreated: another user's Deploy while
// the dialog is open shows up in it.
func TestRevisionsDialog_ReloadsOnRevisionCreated(t *testing.T) {
	url, api := newServer(t)
	if _, err := api.Deploy(context.Background()); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	f := mount(t, url, api)
	f.p.openRevisionsDialog(f.p.compoCtx)
	f.run()

	createScene(t, api, "After the Deploy")
	if _, err := api.Deploy(context.Background()); err != nil {
		t.Fatalf("deploy 2: %v", err)
	}
	f.event(apiv0.Event{Type: apiv0.EventRevisionCreated})
	f.noToasts()

	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-revision-row="2"`) {
		t.Errorf("the open list did not pick up the new Revision: %s", html)
	}
}

// TestDeploy_IsRefusedWhileARollbackRuns: the bar disables Deploy during a
// Rollback, and the handler refuses it too, so the two writes of the running
// configuration never race.
func TestDeploy_IsRefusedWhileARollbackRuns(t *testing.T) {
	url, api := newServer(t)
	deployTwice(t, api)
	f := mount(t, url, api)

	f.p.rollingBack = true
	f.p.deploy(f.p.compoCtx)
	if f.p.deploying {
		t.Error("a Deploy started while a Rollback was running")
	}
}
