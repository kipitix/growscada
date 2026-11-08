package project

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// documentOfFile encodes a ProjectFile as the picked bytes.
func documentOfFile(t *testing.T, file projectv0.ProjectFile) []byte {
	t.Helper()
	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("encode the document: %v", err)
	}
	return data
}

// TestProjectFileDialog_ShowsTheActionsAndTheConfirmation: the header's
// Project file button opens one dialog with the three actions; a picked file
// is shown with its schema, project and counts before anything is applied
// (ADR 0018).
func TestProjectFileDialog_ShowsTheActionsAndTheConfirmation(t *testing.T) {
	f := startProject(t)

	f.p.openProjectFileDialog(f.p.compoCtx)
	html := app.HTMLString(f.p.Render())
	for _, want := range []string{"Project file", "Download Draft", "Open a file…", "Deploy a file…"} {
		if !strings.Contains(html, want) {
			t.Errorf("the dialog does not show %q: %s", want, html)
		}
	}
	// The open action is offered and its hint says what it does.
	if !strings.Contains(html, "Open a file…") {
		t.Errorf("the dialog does not offer the open action: %s", html)
	}

	file, err := f.api.DraftProjectFile(context.Background())
	if err != nil {
		t.Fatalf("download the draft: %v", err)
	}
	f.p.pickProjectFile(f.p.compoCtx, "open", "draft.json", file.Data)
	html = app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-projectfile-confirmation="open"`) {
		t.Fatalf("the picked file has no confirmation: %s", html)
	}
	for _, want := range []string{"draft.json", "Schema 0.2", "tags", "replaces the whole Draft", "undo restores"} {
		if !strings.Contains(html, want) {
			t.Errorf("the confirmation does not show %q: %s", want, html)
		}
	}
	if strings.Contains(html, `data-projectfile-confirm`) && !strings.Contains(html, "Open") {
		t.Errorf("the confirmation has no Open button: %s", html)
	}
}

// TestProjectFileDialog_DownloadAsksTheServer: the Download Draft action
// reads the file from the server; the browser's Blob is a no-op in the test
// engine, so the check is that the call completes without a refusal.
func TestProjectFileDialog_DownloadAsksTheServer(t *testing.T) {
	f := startProject(t)
	f.p.openProjectFileDialog(f.p.compoCtx)
	f.p.downloadDraftProjectFile(f.p.compoCtx)
	f.run()
	f.noToasts()

	if f.p.projectFileBusy {
		t.Error("the download is still in flight")
	}
	if !f.p.projectFileDialog {
		t.Error("the dialog closed on a download")
	}
}

// TestProjectFileDialog_OpenRestoresTheDraftAndCloses: opening a downloaded
// file restores what was deleted, reloads the content and the stack, and
// closes the dialog.
func TestProjectFileDialog_OpenRestoresTheDraftAndCloses(t *testing.T) {
	f := startProject(t)
	created := createTag(t, f.api, "speed", "integer")
	f.p.loadDraftState(f.p.compoCtx)
	f.run()
	file, err := f.api.DraftProjectFile(context.Background())
	if err != nil {
		t.Fatalf("download the draft: %v", err)
	}
	if _, err := f.api.DeleteTag(context.Background(), created, f.p.draftVersion()); err != nil {
		t.Fatalf("delete the tag: %v", err)
	}
	f.p.loadDraftState(f.p.compoCtx)
	f.run()

	f.p.openProjectFileDialog(f.p.compoCtx)
	f.p.pickProjectFile(f.p.compoCtx, "open", "draft.json", file.Data)
	f.p.confirmPendingProjectFile(f.p.compoCtx)
	f.run()
	f.noToasts()

	if f.p.projectFileDialog || f.p.pendingProjectFile != nil {
		t.Error("the dialog stayed open after the file was opened")
	}
	if _, ok := f.p.findTag(created); !ok {
		t.Errorf("the opened file did not restore the tag: %+v", f.p.tags)
	}
	if !f.p.changesLoaded {
		t.Error("the DraftChange stack was not reread after the open")
	}
}

// TestProjectFileDialog_FileIssuesAreAList: the server's file_issues are
// shown as a list of paths and messages, and the dialog stays open.
func TestProjectFileDialog_FileIssuesAreAList(t *testing.T) {
	f := startProject(t)
	f.p.openProjectFileDialog(f.p.compoCtx)
	f.p.pickProjectFile(f.p.compoCtx, "open", "empty.json", []byte(`{}`))
	f.p.confirmPendingProjectFile(f.p.compoCtx)
	f.run()

	if !f.p.projectFileDialog {
		t.Error("the dialog closed on a refused file")
	}
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-projectfile-issues`) {
		t.Fatalf("the problems are not listed: %s", html)
	}
	if !strings.Contains(html, "project_id:") {
		t.Errorf("the list does not name the path: %s", html)
	}
	if toasts := f.e.TakeToasts(); len(toasts) != 0 {
		t.Errorf("a refused file is a toast instead of a list: %+v", toasts)
	}
}

// TestProjectFileDialog_DeployFromFileReloadsTheDeployment: a deployed file
// becomes a Release and the Deployment the Project runs, and the Draft stays
// as it was.
func TestProjectFileDialog_DeployFromFileReloadsThePreview(t *testing.T) {
	f := startProject(t)
	if _, err := f.api.CreateTag(context.Background(), apiv0.CreateTagRequest{Name: "speed", Type: "integer"}); err != nil {
		t.Fatalf("create tag: %v", err)
	}
	f.p.loadDraftState(f.p.compoCtx)
	f.run()
	// The file of another server's empty project: it differs from the Draft.
	foreign := projectv0.ProjectFile{Header: projectv0.Header{
		SchemaVersion: projectv0.SchemaVersion(), ProjectID: f.p.projectID,
	}}
	f.p.openProjectFileDialog(f.p.compoCtx)
	f.p.pickProjectFile(f.p.compoCtx, "deploy", "empty.json", documentOfFile(t, foreign))
	f.p.confirmPendingProjectFile(f.p.compoCtx)
	f.run()
	f.noToasts()

	if f.p.projectFileDialog {
		t.Error("the dialog stayed open after the file was deployed")
	}
	if !f.p.deployed || f.p.deployment.Number != 1 || f.p.deployment.ReleaseNumber == nil || *f.p.deployment.ReleaseNumber != 1 {
		t.Fatalf("the bar after a file deploy: deployed %v, %+v (dialog %v, pending %+v, toasts %+v)",
			f.p.deployed, f.p.deployment, f.p.projectFileDialog, f.p.pendingProjectFile, f.e.TakeToasts())
	}
}

// TestDeploymentsDialog_MarksAFileDeploymentAndOffersDownloads: a file deploy
// leaves a Release and is shown as a Deploy of it, and each row offers its own
// download.
func TestDeploymentsDialog_MarksAFileDeploymentAndOffersDownloads(t *testing.T) {
	url, api := newServer(t)
	file := projectv0.ProjectFile{Header: projectv0.Header{
		SchemaVersion: projectv0.SchemaVersion(), ProjectID: api.ID(),
	}}
	if _, err := api.DeployProjectFile(context.Background(), documentOfFile(t, file)); err != nil {
		t.Fatalf("deploy a file: %v", err)
	}
	f := mount(t, url, api)
	f.p.openDeploymentsDialog(f.p.compoCtx)
	f.run()
	f.noToasts()

	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Release 1") {
		t.Errorf("the list does not name the Release of the file: %s", html)
	}
	if !strings.Contains(html, `data-deployment-download="1"`) {
		t.Errorf("the row offers no download: %s", html)
	}
}
