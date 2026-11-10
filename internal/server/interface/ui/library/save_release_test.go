package library

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
)

// The Library's Save → Commit → Release flow of task 72_7: Ctrl+S, the status
// of the list, Discard and Revert, a repeated publication and the read-only
// view of a Commit.

// saveWithUI fixes the Draft as a Commit through the Save dialog and consumes
// its confirmation toast.
func saveWithUI(t *testing.T, f *fixture, title string) {
	t.Helper()
	f.l.saveAuthor, f.l.saveTitle = "Ada", title
	f.l.saveCommit()
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "saved") {
		t.Fatalf("the Save confirmation: %+v", got)
	}
}

// TestSaveFromShortcut_OpensTheDialogOnlyWithChanges: Ctrl+S opens the Save
// dialog when the Draft moved since its last Commit, nothing otherwise
// (task 72_7; ADR 0021).
func TestSaveFromShortcut_OpensTheDialogOnlyWithChanges(t *testing.T) {
	f, _ := selected(t)
	f.run()
	if !f.l.hasUnsavedChanges() {
		t.Fatal("a Draft written but never saved reports no unsaved changes")
	}
	f.l.saveFromShortcut(f.l.compoCtx)
	if !f.l.saveDialog {
		t.Fatal("Ctrl+S did not open the Save dialog with unsaved changes")
	}
	saveWithUI(t, f, "First save")

	f.l.saveFromShortcut(f.l.compoCtx)
	if f.l.saveDialog {
		t.Error("Ctrl+S opened the Save dialog with nothing to save")
	}
}

// TestSaveCommit_ConflictRereadsTheDraft: another tab edits the Draft before
// this one saves: the 409 is a toast, and the Draft, the stack and the history
// are read again so the next Save states the server's Version (task 72_7).
func TestSaveCommit_ConflictRereadsTheDraft(t *testing.T) {
	f, id := selected(t)
	f.run()
	wt := f.widgetType(id)
	if _, err := f.api.UpdateWidgetType(context.Background(), id, apiv0.UpdateWidgetTypeRequest{
		Name: "Theirs", HtmlTemplate: wt.HtmlTemplate, Script: wt.Script, ScriptLanguage: wt.ScriptLanguage,
		DefaultWidth: wt.DefaultWidth, DefaultHeight: wt.DefaultHeight, InputPorts: wt.InputPorts, Version: wt.Version,
	}); err != nil {
		t.Fatalf("update widget type: %v", err)
	}

	f.l.saveAuthor, f.l.saveTitle = "Ada", "Stale save"
	f.l.saveCommit()
	f.run()
	if got := f.oneToast(); got.Status != http.StatusConflict {
		t.Fatalf("the toast %+v: want the server's 409", got)
	}
	if got, want := f.l.draftVersion(), f.widgetType(id).Version; got != want {
		t.Errorf("the known Version %d, the server's %d: the conflict did not reread the Draft", got, want)
	}

	// The next Save starts from the Version the conflict left.
	f.l.saveAuthor, f.l.saveTitle = "Ada", "After the conflict"
	f.l.saveCommit()
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "saved") {
		t.Fatalf("the second Save: %+v", got)
	}
}

// TestDiscard_ResetsToTheLastCommitAndClearsTheIndicator: Discard drops the
// unsaved changes, puts the Draft back to its last Commit and turns the
// indicator off even though the Draft's Version rose (task 72_7; ADR 0021).
func TestDiscard_ResetsToTheLastCommitAndClearsTheIndicator(t *testing.T) {
	f, kept := selected(t)
	f.run()
	saveWithUI(t, f, "First save")

	added := createWidgetType(t, f.api, "Added")
	f.event(apiv0.Event{Type: apiv0.EventWidgetTypeCreated, ID: added.String(), LibraryID: f.l.libraryID.String()})
	if !f.l.hasUnsavedChanges() {
		t.Fatal("the change after the Commit is not unsaved")
	}

	f.l.doDiscard(f.l.compoCtx)
	f.run()
	f.noToasts()
	if f.l.hasUnsavedChanges() {
		t.Error("the Draft still reports unsaved changes after a Discard")
	}
	list, err := f.api.ListWidgetTypes(context.Background())
	if err != nil {
		t.Fatalf("server widget types: %v", err)
	}
	if len(list) != 1 || list[0].ID != kept {
		t.Errorf("the server's Draft after the Discard: %+v, want only the Commit's", list)
	}
	if len(f.l.shownTypes()) != 1 || f.l.shownTypes()[0].ID != kept {
		t.Errorf("the shown Draft after the Discard: %+v", f.l.shownTypes())
	}
}

// TestRevert_ResetsToTheLatestReleaseAndClearsTheIndicator: Revert goes back
// to the published state; when that Release published the last Commit, nothing
// is unsaved either (task 72_7; ADR 0021).
func TestRevert_ResetsToTheLatestReleaseAndClearsTheIndicator(t *testing.T) {
	f, kept := selected(t)
	f.run()
	releaseNow(t, f)
	if f.l.hasUnsavedChanges() {
		t.Fatal("the Draft of a fresh Release reports unsaved changes")
	}

	added := createWidgetType(t, f.api, "Added")
	f.event(apiv0.Event{Type: apiv0.EventWidgetTypeCreated, ID: added.String(), LibraryID: f.l.libraryID.String()})
	if !f.l.hasUnsavedChanges() {
		t.Fatal("the change after the Release is not unsaved")
	}

	f.l.doRevert(f.l.compoCtx)
	f.run()
	f.noToasts()
	if f.l.hasUnsavedChanges() {
		t.Error("the Draft still reports unsaved changes after a Revert to its last Commit's Release")
	}
	if len(f.l.shownTypes()) != 1 || f.l.shownTypes()[0].ID != kept {
		t.Errorf("the shown Draft after the Revert: %+v", f.l.shownTypes())
	}
}

// TestPublishRelease_RepeatedPublicationIsAToast: a Commit of the history is
// published from its dialog, and a second publication of the same Commit is
// the server's 409 as a toast (task 72_7; ADR 0021).
func TestPublishRelease_RepeatedPublicationIsAToast(t *testing.T) {
	f, _ := selected(t)
	f.run()
	state, err := f.api.DraftState(context.Background())
	if err != nil {
		t.Fatalf("DraftState: %v", err)
	}
	if _, err := f.api.SaveCommit(context.Background(), apiv0.CreateCommitRequest{
		Version: state.Version, Title: "First save", Author: "Ada",
	}); err != nil {
		t.Fatalf("SaveCommit: %v", err)
	}
	f.l.loadCommits(f.l.compoCtx)
	f.run()
	f.l.saveAuthor = "Ada"

	f.l.openReleaseDialog(f.l.commits[0])
	if !f.l.releaseDialog || f.l.releaseCommit != 1 {
		t.Fatalf("the release dialog: open %t, commit %d", f.l.releaseDialog, f.l.releaseCommit)
	}
	f.l.releaseAuthor = "Ada"
	f.l.publishRelease()
	f.run()
	if got := f.oneToast(); !strings.Contains(got.Title, "Release 1 published") {
		t.Fatalf("the publication toast: %+v", got)
	}
	if len(f.l.releases) != 1 || f.l.releases[0].Number != 1 {
		t.Fatalf("the releases after the publication: %+v", f.l.releases)
	}

	// Another tab may have published it meanwhile: the commit this one holds
	// still says it is unpublished.
	f.l.openReleaseDialog(apiv0.CommitResponse{Number: 1, Title: "First save"})
	f.l.publishRelease()
	f.run()
	if got := f.oneToast(); got.Status != http.StatusConflict {
		t.Errorf("the repeated publication: %+v, want the server's 409", got)
	}
}

// TestCommitView_ShowsTheSnapshotReadOnly: a Commit's WidgetTypes are shown
// from its snapshot, never from the Draft, and cannot be edited (task 72_7).
func TestCommitView_ShowsTheSnapshotReadOnly(t *testing.T) {
	f, id := selected(t)
	f.run()
	saveWithUI(t, f, "First save")

	f.l.openCommitsDialog()
	f.run()
	if html := app.HTMLString(f.l.Render()); !strings.Contains(html, `data-commit-row="1"`) {
		t.Fatalf("the history does not show Commit 1: %s", html)
	}
	f.l.viewCommit(1)
	f.run()
	f.noToasts()
	if f.l.viewingCommit != 1 || !f.l.readOnly() {
		t.Errorf("the Commit view: viewingCommit %d, read-only %t", f.l.viewingCommit, f.l.readOnly())
	}
	if types := f.l.shownTypes(); len(types) != 1 || types[0].ID != id {
		t.Errorf("the Commit's WidgetTypes: %+v", types)
	}
	if title := f.l.listTitle(); !strings.Contains(title, "Commit 1") {
		t.Errorf("the list title %q: want the Commit named", title)
	}
	// A Commit is not the Draft: the stack commands and Save say so too
	// (task 72_7).
	if f.l.canUndo() || f.l.canRedo() {
		t.Errorf("the Commit view offers a stack step: %+v", f.l.changes)
	}
	if title := f.l.undoTitle(); !strings.Contains(title, "open the Draft to edit it") {
		t.Errorf("the undo title %q: want the Commit named as read-only", title)
	}
	if title := f.l.redoTitle(); !strings.Contains(title, "open the Draft to edit it") {
		t.Errorf("the redo title %q: want the Commit named as read-only", title)
	}
	if html := app.HTMLString(app.Div().Body(f.l.renderCommitBar()...)); !strings.Contains(html, "open the Draft to edit it") {
		t.Errorf("the Save button of the Commit view: %s", html)
	}

	f.l.view(0)
	if f.l.readOnly() || f.l.viewingCommit != 0 || len(f.l.shownTypes()) != 1 {
		t.Error("the Draft is not shown again after the Commit view")
	}
}

// TestLibraryState_ListsTheThreeStates: the header's state of the selected
// library follows the Draft: unsaved changes, an unpublished Commit, then
// published (task 72_7; task 73; ADR 0021).
func TestLibraryState_ListsTheThreeStates(t *testing.T) {
	f, _ := selected(t)
	f.run()
	status := func() string { return app.HTMLString(f.l.renderLibraryStatus()) }
	if html := status(); !strings.Contains(html, "unsaved changes") {
		t.Errorf("the header does not mark the unsaved Draft: %s", html)
	}

	saveWithUI(t, f, "First save")
	f.run()
	if html := status(); !strings.Contains(html, "not published yet") {
		t.Errorf("the header does not mark the unpublished Commit: %s", html)
	}

	releaseNow(t, f)
	if html := status(); !strings.Contains(html, "published") {
		t.Errorf("the header does not mark the published state: %s", html)
	}
}

// TestReleaseRow_ShowsItsCounts: a Release of the list names the Commit it
// published, its Author, time and the WidgetTypes it copied (task 72_7).
func TestReleaseRow_ShowsItsCounts(t *testing.T) {
	f, _ := selected(t)
	f.run()
	releaseNow(t, f)
	if len(f.l.releases) != 1 {
		t.Fatalf("the releases: %+v", f.l.releases)
	}
	html := app.HTMLString(f.l.renderReleaseRow(f.l.releases[0]))
	if !strings.Contains(html, "commit #1") || !strings.Contains(html, "1 widget types") || !strings.Contains(html, "Ada") {
		t.Errorf("the release row: %s", html)
	}
}

// TestServerEvents_AnotherLibraryLeavesTheSelectedAlone: a Commit, a Release
// or a note of another library is not the selected one's business (task 72_7).
func TestServerEvents_AnotherLibraryLeavesTheSelectedAlone(t *testing.T) {
	url, api := newServer(t)
	other, err := api.CreateLibrary(context.Background(), apiv0.CreateLibraryRequest{Name: "Other"})
	if err != nil {
		t.Fatalf("create the other library: %v", err)
	}
	f := mount(t, url, api)
	f.run()
	f.l.releases = []apiv0.LibraryReleaseResponse{{Number: 99}}
	f.l.commits = []apiv0.CommitResponse{{Number: 99}}

	for _, ev := range []apiv0.Event{
		{Type: apiv0.EventCommitCreated, ID: other.ID.String(), Commit: 7},
		{Type: apiv0.EventLibraryReleased, ID: other.ID.String(), Release: 1},
		{Type: apiv0.EventLibraryReleaseNoteUpdated, ID: other.ID.String(), Release: 1},
	} {
		f.event(ev)
		if len(f.l.releases) != 1 || f.l.releases[0].Number != 99 {
			t.Fatalf("%s of another library reloaded the releases: %+v", ev.Type, f.l.releases)
		}
		if len(f.l.commits) != 1 || f.l.commits[0].Number != 99 {
			t.Fatalf("%s of another library reloaded the history: %+v", ev.Type, f.l.commits)
		}
	}
}

// TestWidgetTypeEvent_DoesNotReloadTheReleasesWithContent: a WidgetType of the
// selected Draft changes the list and the unsaved flag, never the Releases
// with their copies (task 72_7).
func TestWidgetTypeEvent_DoesNotReloadTheReleasesWithContent(t *testing.T) {
	f, id := selected(t)
	f.run()
	releaseNow(t, f)
	f.l.releases = []apiv0.LibraryReleaseResponse{{Number: 99}}

	f.event(apiv0.Event{Type: apiv0.EventWidgetTypeUpdated, ID: id.String(), LibraryID: f.l.libraryID.String()})
	if len(f.l.releases) != 1 || f.l.releases[0].Number != 99 {
		t.Errorf("a WidgetType event reloaded the Releases with their content: %+v", f.l.releases)
	}
	if len(f.l.widgetTypes) != 1 || f.l.widgetTypes[0].ID != id {
		t.Errorf("a WidgetType event did not reread the Draft: %+v", f.l.widgetTypes)
	}
}

// TestServerEvents_ProjectResyncLeavesTheLibraryAlone: the Bar's stream may
// follow a Project while Library is shown; a resync of that stream makes none
// of the Library's lists stale (task 72_7).
func TestServerEvents_ProjectResyncLeavesTheLibraryAlone(t *testing.T) {
	f, _ := selected(t)
	f.run()
	f.l.releases = []apiv0.LibraryReleaseResponse{{Number: 99}}
	f.l.commits = []apiv0.CommitResponse{{Number: 99}}

	f.event(apiv0.Event{Type: eventlog.EventProjectResync})
	if len(f.l.releases) != 1 || f.l.releases[0].Number != 99 {
		t.Errorf("a project resync reloaded the releases: %+v", f.l.releases)
	}
	if len(f.l.commits) != 1 || f.l.commits[0].Number != 99 {
		t.Errorf("a project resync reloaded the history: %+v", f.l.commits)
	}
}

// TestCommitTitleEvent_RereadsTheReleases: a Library Release names its Commit
// by the Commit's current title — its own copy carries none —, so a rename
// changes the Release list too (task 72_7; ADR 0021).
func TestCommitTitleEvent_RereadsTheReleases(t *testing.T) {
	f, _ := selected(t)
	f.run()
	releaseNow(t, f)
	if len(f.l.releases) != 1 || f.l.releases[0].Title == nil || *f.l.releases[0].Title != "Save" {
		t.Fatalf("the released Commit's title: %+v", f.l.releases)
	}
	if _, err := f.api.RenameCommit(context.Background(), 1, apiv0.RenameCommitRequest{Title: "Renamed"}); err != nil {
		t.Fatalf("rename the Commit: %v", err)
	}

	f.event(apiv0.Event{Type: apiv0.EventCommitTitleUpdated, ID: f.l.libraryID.String(), Commit: 1})
	if len(f.l.releases) != 1 || f.l.releases[0].Title == nil || *f.l.releases[0].Title != "Renamed" {
		t.Errorf("a rename of the Commit did not reread the Releases: %+v", f.l.releases)
	}
}

// TestDiscard_AnotherTabsDiscardClearsTheIndicator: a Discard performed
// elsewhere arrives as the Draft's content events; the indicator is the flag
// the server answers (task 74; ADR 0016, amendment 2026-11-10).
func TestDiscard_AnotherTabsDiscardClearsTheIndicator(t *testing.T) {
	f, _ := selected(t)
	f.run()
	saveWithUI(t, f, "First save")

	createWidgetType(t, f.api, "Added")
	f.l.loadUnsavedChanges(f.l.compoCtx)
	f.run()
	if !f.l.hasUnsavedChanges() {
		t.Fatal("the change after the Commit is not unsaved")
	}

	if _, err := f.api.Discard(context.Background()); err != nil {
		t.Fatalf("another tab's Discard: %v", err)
	}
	// The events of the Discard reach this tab: the WidgetType it removed is
	// one.
	f.event(apiv0.Event{
		Type: apiv0.EventWidgetTypeDeleted, ID: uuid.NewString(), LibraryID: f.l.libraryID.String(),
	})
	if f.l.hasUnsavedChanges() {
		t.Errorf("the indicator stays after another tab's Discard")
	}
	if html := app.HTMLString(app.Div().Body(f.l.renderCommitBar()...)); strings.Contains(html, "data-commit-unsaved") {
		t.Errorf("the header still marks unsaved changes: %s", html)
	}
}
