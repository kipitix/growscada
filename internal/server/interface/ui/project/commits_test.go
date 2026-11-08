package project

import (
	"context"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// TestSaveCommit_HistoryRowAndIndicator: Save fixes the Draft as Commit 1, the
// history shows it and the unsaved-changes indicator goes off (task 72_2;
// ADR 0021).
func TestSaveCommit_HistoryRowAndIndicator(t *testing.T) {
	f := startProject(t)
	f.run()
	if !f.p.commitsLoaded {
		t.Fatal("the history was not loaded")
	}
	if !f.p.hasUnsavedChanges() {
		t.Fatalf("a fresh Draft reports no unsaved changes against no Commit")
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-commit-unsaved`) {
		t.Errorf("the header does not mark unsaved changes: %s", html)
	}

	f.p.openSaveDialog(f.p.compoCtx)
	f.p.saveAuthor = "Ada"
	f.p.saveTitle = "First save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()

	f.noToasts()
	if len(f.p.commits) != 1 || f.p.commits[0].Number != 1 || f.p.commits[0].Author != "Ada" {
		t.Fatalf("the history: %+v", f.p.commits)
	}
	if f.p.hasUnsavedChanges() {
		t.Errorf("the Draft still reports unsaved changes after a Save")
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-commit-saved`) {
		t.Errorf("the header does not mark the Draft saved: %s", html)
	}

	f.p.openCommitsDialog(f.p.compoCtx)
	f.run()
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-commit-row="1"`) {
		t.Errorf("the history does not show Commit 1: %s", html)
	}
}

// TestSaveCommit_ConflictRereadsAndRename: a Save of a stale Version is a
// conflict that rereads the Draft, and a Commit's title is renamed in place
// (task 72_2; ADR 0021).
func TestSaveCommit_ConflictRereadsAndRename(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"
	f.p.saveTitle = "First save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()
	f.noToasts()

	// Another tab writes the Draft: the Version rises above the one this tab
	// saved at.
	if _, err := f.api.CreateTag(context.Background(), apiv0.CreateTagRequest{Name: "speed", Type: "integer"}); err != nil {
		t.Fatalf("another tab's write: %v", err)
	}
	// This tab still states the Version it knew: forgetting it makes the next
	// Save state 0, which the server refuses as stale.
	f.p.forgetDraftVersion()
	f.p.saveTitle = "Stale save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()

	if got := f.oneToast(); got.Status != 409 {
		t.Fatalf("a stale Save: toast %+v, want a 409", got)
	}
	if f.p.draftVersion() <= f.p.lastCommitVersion() {
		t.Errorf("the conflict did not reread the Draft: version %d, last commit %d",
			f.p.draftVersion(), f.p.lastCommitVersion())
	}
	f.p.saveDialog = false

	f.p.startRenamingCommit(1, "First save")
	f.p.renameCommitTitle = "Before the demo"
	f.p.renameCommit(f.p.compoCtx)
	f.run()
	f.noToasts()
	if len(f.p.commits) != 1 || f.p.commits[0].Title != "Before the demo" {
		t.Errorf("the renamed history: %+v", f.p.commits)
	}
	if f.p.renamingCommit != 0 {
		t.Errorf("the inline rename editor stayed open")
	}
}
