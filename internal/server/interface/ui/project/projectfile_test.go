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
	// The lock is held here: the open action is enabled and its hint must not
	// tell the user to check the Draft out.
	if strings.Contains(html, "Replace the whole Draft with a file; can be undone — Check the Draft out") {
		t.Errorf("the open action carries the read-only hint while the lock is held: %s", html)
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

// TestProjectFileDialog_OpenIsDisabledWithoutTheLock: the Draft write is
// gated like every other; the deploy of a file stays available.
func TestProjectFileDialog_OpenIsDisabledWithoutTheLock(t *testing.T) {
	f := startProject(t)
	f.p.editLock, f.p.lockLoaded = apiv0.EditLockResponse{}, true

	f.p.openProjectFileDialog(f.p.compoCtx)
	html := app.HTMLString(f.p.Render())
	if strings.Contains(html, `data-projectfile-input="open"`) {
		t.Errorf("the open input is offered without the EditLock: %s", html)
	}
	if !strings.Contains(html, `data-projectfile-input="deploy"`) {
		t.Errorf("the deploy input is disabled without the EditLock: %s", html)
	}
	if !strings.Contains(html, "Check the Draft out") {
		t.Errorf("the disabled open has no hint: %s", html)
	}
}

// TestProjectFileDialog_DeployFromFileReloadsThePreview: a deployed file
// becomes the Revision the Project runs and leaves the Draft as it was.
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
	if f.p.deployPreview.CurrentRevision == nil || f.p.deployPreview.CurrentRevision.Number != 1 {
		var current any
		if f.p.deployPreview.CurrentRevision != nil {
			current = *f.p.deployPreview.CurrentRevision
		}
		t.Fatalf("the bar after a file deploy: %+v, current %+v (dialog %v, pending %+v, toasts %+v)",
			f.p.deployPreview, current, f.p.projectFileDialog, f.p.pendingProjectFile, f.e.TakeToasts())
	}
	if f.p.deployPreview.Unchanged {
		t.Error("the file deploy reported the Draft as deployed")
	}
}

// TestRevisionsDialog_MarksAFileRevisionAndOffersDownloads: the cause file is
// shown as "Deployed from file", and each row offers its own download.
func TestRevisionsDialog_MarksAFileRevisionAndOffersDownloads(t *testing.T) {
	url, api := newServer(t)
	file := projectv0.ProjectFile{Header: projectv0.Header{
		SchemaVersion: projectv0.SchemaVersion(), ProjectID: api.ID(),
	}}
	if _, err := api.DeployProjectFile(context.Background(), documentOfFile(t, file)); err != nil {
		t.Fatalf("deploy a file: %v", err)
	}
	f := mount(t, url, api)
	f.p.openRevisionsDialog(f.p.compoCtx)
	f.run()
	f.noToasts()

	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Deployed from file") {
		t.Errorf("the list does not name the file cause: %s", html)
	}
	if !strings.Contains(html, `data-revision-download="1"`) {
		t.Errorf("the row offers no download: %s", html)
	}
}
