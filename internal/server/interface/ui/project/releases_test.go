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
	f.noToasts()

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
