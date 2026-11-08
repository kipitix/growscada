package project

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// TestRelease_FromTheHistory: a Commit is published from the history, the
// Releases dialog lists it and its note is changed in place (task 72_3;
// ADR 0021).
func TestRelease_FromTheHistory(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"
	f.p.saveTitle = "First save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()
	f.e.TakeToasts() // the Save's own confirmation

	f.p.openCommitsDialog(f.p.compoCtx)
	f.run()
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-commit-release="1"`) {
		t.Fatalf("the history offers no release of Commit 1: %s", html)
	}

	f.p.openReleaseDialog(1)
	f.p.releaseNote = " for the demo "
	f.p.releaseAuthor = "Grace"
	f.p.publishRelease(f.p.compoCtx)
	f.run()

	if toasts := f.e.TakeToasts(); len(toasts) != 1 {
		t.Errorf("the publication showed %d toasts, want one", len(toasts))
	}
	if len(f.p.releases) != 1 || f.p.releases[0].Number != 1 || f.p.releases[0].Note != "for the demo" {
		t.Fatalf("the releases: %+v", f.p.releases)
	}
	if f.p.releaseDialog || f.p.releaseCommit != 0 {
		t.Errorf("the release dialog stayed open")
	}
	if len(f.p.commits) != 1 || !f.p.commits[0].Released {
		t.Errorf("the history does not mark the published commit: %+v", f.p.commits)
	}
	f.p.openCommitsDialog(f.p.compoCtx)
	f.run()
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-commit-published="1"`) {
		t.Errorf("the history shows no published mark: %s", html)
	}
	f.p.commitsDialog = false

	f.p.openReleasesDialog(f.p.compoCtx)
	f.run()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-release-row="1"`) || !strings.Contains(html, `data-release-edit-note="1"`) {
		t.Errorf("the releases dialog does not show Release 1: %s", html)
	}

	f.p.startEditingReleaseNote(1, "for the demo")
	f.p.releaseNoteDraft = "after the demo"
	f.p.saveReleaseNote(f.p.compoCtx)
	f.run()
	f.noToasts()
	if len(f.p.releases) != 1 || f.p.releases[0].Note != "after the demo" {
		t.Errorf("the changed note: %+v", f.p.releases)
	}
	if f.p.editingRelease != 0 {
		t.Errorf("the note editor stayed open")
	}

	// A Commit is published at most once: a second publication is the
	// server's 409 with a toast, and the list still holds one Release.
	f.p.openReleaseDialog(1)
	f.p.releaseAuthor = "Grace"
	f.p.publishRelease(f.p.compoCtx)
	f.run()
	if toasts := f.e.TakeToasts(); len(toasts) != 1 {
		t.Errorf("a second release of Commit 1 showed %d toasts, want one", len(toasts))
	}
	if len(f.p.releases) != 1 || f.p.releases[0].Number != 1 {
		t.Errorf("a second release of Commit 1 was stored: %+v", f.p.releases)
	}
}

// TestReleasesDialog_EmptySaysWhatToDoFirst: with no Release, the Deploy list
// says a Release has to be saved and published before anything is deployed
// (task 72_6).
func TestReleasesDialog_EmptySaysWhatToDoFirst(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.openReleasesDialog(f.p.compoCtx)
	f.run()
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, "Save the Draft, then publish a Release") {
		t.Errorf("the empty Release list does not say what to do first: %s", html)
	}
}

// TestRelease_ShortcutsWaitForTheDialogs: the Releases dialogs hold the Draft's
// shortcuts like the mode's other dialogs (task 72_3; ADR 0021).
func TestRelease_ShortcutsWaitForTheDialogs(t *testing.T) {
	f := startProject(t)
	f.run()
	if !f.p.keyActionAllowed() {
		t.Fatal("a shortcut is refused without a dialog")
	}
	f.p.releasesDialog = true
	if f.p.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the Releases dialog")
	}
	f.p.releasesDialog, f.p.releaseDialog = false, true
	if f.p.keyActionAllowed() {
		t.Errorf("a shortcut acts behind the release dialog")
	}
}
