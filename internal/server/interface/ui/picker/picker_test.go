package picker

import (
	"context"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/uitest"
	"github.com/kipitix/growscada/internal/server/servertest"
)

// The selection page runs against the real REST API over Postgres.

var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// mountOnHost lets go-app mount a Picker on the host (see uitest).
type mountOnHost struct{ *Picker }

func (m mountOnHost) OnPreRender(ctx app.Context) { m.OnMount(ctx) }

// fixture is a mounted selection page, the selections and openings it
// reported, and a second client of the same server.
type fixture struct {
	t        *testing.T
	p        *Picker
	e        *uitest.Engine
	api      *apiclient.Client
	selected []uuid.UUID
	opened   []uuid.UUID
}

func start(t *testing.T, source func(*apiclient.Client) Source, key string, arrange func(api *apiclient.Client)) *fixture {
	t.Helper()
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	api := apiclient.New(srv.URL, srv.Client())
	if arrange != nil {
		arrange(api)
	}
	f := &fixture{t: t, api: api}
	f.p = New(source(apiclient.New(srv.URL, srv.Client())), key,
		func(id uuid.UUID, _ string) { f.selected = append(f.selected, id) },
		func(id uuid.UUID) { f.opened = append(f.opened, id) })
	f.e = uitest.Load(t, mountOnHost{f.p})
	return f
}

func startProjects(t *testing.T, arrange func(api *apiclient.Client)) *fixture {
	t.Helper()
	return start(t, NewProjectSource, ProjectKey, arrange)
}

func startLibraries(t *testing.T, arrange func(api *apiclient.Client)) *fixture {
	t.Helper()
	return start(t, NewLibrarySource, LibraryKey, arrange)
}

func (f *fixture) run() { f.e.ConsumeAll() }

func (f *fixture) create(name, description string) uuid.UUID {
	f.t.Helper()
	created, err := f.api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: name, Description: description})
	if err != nil {
		f.t.Fatalf("create project: %v", err)
	}
	return created.ID
}

// names returns the names listed, in the order shown.
func (f *fixture) names() []string {
	shown := f.p.Displayed()
	names := make([]string, len(shown))
	for i, item := range shown {
		names[i] = item.Name
	}
	return names
}

// selectByName shows the details of the entity named name.
func (f *fixture) selectByName(name string) Item {
	f.t.Helper()
	for _, item := range f.p.Displayed() {
		if item.Name == name {
			f.p.selectItem(f.p.compoCtx, item)
			f.run()
			return item
		}
	}
	f.t.Fatalf("no %q in %v", name, f.names())
	return Item{}
}

// TestPicker_ListsSortedWithDescriptionAndState: the page reads the live
// entities on mount and sorts them by name; the list shows names and the state
// dot, the details the description and the state in words (task 73, task 82).
func TestPicker_ListsSortedWithDescriptionAndState(t *testing.T) {
	f := startProjects(t, func(api *apiclient.Client) {
		zulu, err := api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: "zulu", Description: "The last line"})
		if err != nil {
			t.Fatalf("create project: %v", err)
		}
		// A project whose Draft was never written has nothing to save: only
		// the one with a change carries the badge (task 73).
		if _, err := api.Project(zulu.ID).CreateTag(context.Background(), apiv0.CreateTagRequest{Name: "pump.flow", Type: "integer"}); err != nil {
			t.Fatalf("create tag: %v", err)
		}
		if _, err := api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: "Alpha"}); err != nil {
			t.Fatalf("create project: %v", err)
		}
	})
	f.run()

	if got := f.names(); len(got) != 3 || got[0] != "Alpha" || got[1] != servertest.MainProjectName || got[2] != "zulu" {
		t.Fatalf("listed %v, want them sorted by name", got)
	}
	var zulu, alpha Item
	for _, item := range f.p.items {
		switch item.Name {
		case "zulu":
			zulu = item
		case "Alpha":
			alpha = item
		}
	}
	if zulu.Description != "The last line" || zulu.State != StateUnsavedChanges || zulu.StateKind != "unsaved" {
		t.Errorf("zulu: %+v, want its description and unsaved changes", zulu)
	}
	if alpha.Description != "" || alpha.State != "" {
		t.Errorf("alpha: %+v, want no description and no badge", alpha)
	}

	// The list is names and the dot of the state; the details carry the rest.
	list := app.HTMLString(f.p.Render())
	for _, want := range []string{`data-picker-row`, `data-picker-dot="unsaved"`, "Projects", `data-picker-selected="0"`} {
		if !strings.Contains(list, want) {
			t.Errorf("the list does not show %q: %s", want, list)
		}
	}

	f.selectByName("zulu")
	html := app.HTMLString(f.p.Render())
	for _, want := range []string{`data-picker-name`, "The last line", "Unsaved changes", `data-screen="details"`} {
		if !strings.Contains(html, want) {
			t.Errorf("the details of zulu do not show %q: %s", want, html)
		}
	}

	f.selectByName("Alpha")
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, emptyDescription) {
		t.Errorf("the details of alpha do not say there is no description: %s", html)
	}
}

// TestPicker_OpenReportsTheEntity: opening a row reports it to the mode and
// keeps it under the LocalStorage key.
func TestPicker_OpenReportsTheEntity(t *testing.T) {
	f := startProjects(t, nil)
	f.run()
	f.p.open(f.p.compoCtx, servertest.MainProject)
	f.run()

	if len(f.opened) != 1 || f.opened[0] != servertest.MainProject {
		t.Errorf("opened %v, want Main", f.opened)
	}
	if len(f.selected) == 0 || f.selected[len(f.selected)-1] != servertest.MainProject {
		t.Errorf("selected %v, want Main", f.selected)
	}
	if got := f.p.Name(servertest.MainProject); got != servertest.MainProjectName {
		t.Errorf("the name reported: %q", got)
	}
}

// TestPicker_CreateOpensTheNewEntity: Create puts a draft row in the list and
// its name field in the details; the server is the only validator, and a
// created entity opens at once (task 82).
func TestPicker_CreateOpensTheNewEntity(t *testing.T) {
	f := startProjects(t, nil)
	f.run()

	f.p.startCreate(f.p.compoCtx)
	f.run()
	if !f.p.creating || f.p.screenName() != screenDetails {
		t.Fatalf("the draft is not open: %+v", f.p)
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, `data-picker-draft`) ||
		!strings.Contains(html, `data-picker-create-name`) {
		t.Errorf("the page does not show the draft: %s", html)
	}

	f.p.createName = "Line 1"
	f.p.commitCreate(f.p.compoCtx)
	f.run()

	if f.p.creating {
		t.Errorf("the draft is still open: %q", f.p.createError)
	}
	created, ok, err := f.api.FindProjectByName(context.Background(), "Line 1")
	if err != nil || !ok {
		t.Fatalf("Line 1 on the server: %v, %v", ok, err)
	}
	if len(f.opened) != 1 || f.opened[0] != created.ID {
		t.Errorf("opened %v, want the created project", f.opened)
	}
	if len(f.p.items) != 2 {
		t.Errorf("listed %d projects after the create, want 2", len(f.p.items))
	}
}

// TestPicker_CreateEmptyNameDropsTheDraft: a draft left without a name is no
// request at all (task 82).
func TestPicker_CreateEmptyNameDropsTheDraft(t *testing.T) {
	f := startProjects(t, nil)
	f.run()

	f.p.startCreate(f.p.compoCtx)
	f.p.createName = "   "
	f.p.commitCreate(f.p.compoCtx)
	f.run()

	if f.p.creating || f.p.createError != "" {
		t.Errorf("the draft: creating %t, %q", f.p.creating, f.p.createError)
	}
	if len(f.p.items) != 1 {
		t.Errorf("listed %d projects, want Main only", len(f.p.items))
	}
}

// TestPicker_ChoosingAnEntityDropsTheDraft: picking another entity while a
// new one is being created abandons the draft, which was never sent (task 82).
func TestPicker_ChoosingAnEntityDropsTheDraft(t *testing.T) {
	f := startProjects(t, nil)
	f.run()

	f.p.startCreate(f.p.compoCtx)
	f.p.createName = "Never sent"
	main := f.selectByName(servertest.MainProjectName)

	if f.p.creating || f.p.createName != "" || f.p.createError != "" {
		t.Errorf("the draft survived a selection: %+v", f.p)
	}
	if f.p.SelectedID() != main.ID {
		t.Errorf("selected %v, want Main", f.p.SelectedID())
	}
	if len(f.p.items) != 1 {
		t.Errorf("the draft reached the server: %+v", f.p.items)
	}
}

// TestPicker_CreateSelectsTheDraftAndCancelRestores: the draft row is the
// list's selection while it is open, and Cancel brings the previous selection
// back (task 82).
func TestPicker_CreateSelectsTheDraftAndCancelRestores(t *testing.T) {
	f := startProjects(t, nil)
	f.run()
	main := f.selectByName(servertest.MainProjectName)

	f.p.startCreate(f.p.compoCtx)
	f.run()

	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-picker-draft="1"`) {
		t.Fatalf("the draft row is not shown: %s", html)
	}
	if got := strings.Count(html, `data-picker-selected="1"`); got != 1 {
		t.Errorf("the list marks %d rows selected with a draft open, want the draft only", got)
	}
	if f.p.SelectedID() != main.ID {
		t.Errorf("the draft moved the selection to %v", f.p.SelectedID())
	}

	f.p.cancelCreate(f.p.compoCtx)
	f.run()

	html = app.HTMLString(f.p.Render())
	if f.p.creating || f.p.SelectedID() != main.ID {
		t.Errorf("after Cancel: creating %t, selected %v", f.p.creating, f.p.SelectedID())
	}
	if strings.Contains(html, "data-picker-draft") {
		t.Errorf("the draft row survived Cancel: %s", html)
	}
	if got := strings.Count(html, `data-picker-selected="1"`); got != 1 {
		t.Errorf("the list marks %d rows selected after Cancel, want the previous one", got)
	}
	if !strings.Contains(html, `data-screen="list"`) {
		t.Errorf("Cancel left the phone on the details screen: %s", html)
	}
}

// TestPicker_EditErrorStaysInTheField: a refusal of the server — here a name
// taken — is shown under the field, which keeps the text and stays open
// (task 82).
func TestPicker_EditErrorStaysInTheField(t *testing.T) {
	f := startProjects(t, func(api *apiclient.Client) {
		if _, err := api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: "Line 1"}); err != nil {
			t.Fatalf("create project: %v", err)
		}
	})
	f.run()

	f.selectByName(servertest.MainProjectName)
	f.p.startEdit(f.p.compoCtx, fieldName)
	f.p.value = "Line 1"
	f.p.commitEdit(f.p.compoCtx)
	f.run()

	if f.p.field != fieldName || f.p.editError == "" || !strings.Contains(f.p.editError, "taken") {
		t.Fatalf("field %q, error %q; want the refusal in the open field", f.p.field, f.p.editError)
	}
	if f.p.value != "Line 1" {
		t.Errorf("the input was lost: %q", f.p.value)
	}
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-picker-error`) || !strings.Contains(html, "taken") || !strings.Contains(html, `value="Line 1"`) {
		t.Errorf("the field does not show the refusal and the input: %s", html)
	}
	if got, err := f.api.GetProject(context.Background(), servertest.MainProject); err != nil || got.Name != servertest.MainProjectName {
		t.Errorf("the refused edit changed the project: %+v, %v", got, err)
	}
}

// TestPicker_SelectingAnotherEntityDropsTheEdit: the inline field belongs to
// the entity it was opened on; picking another one closes it, with the text
// and any refusal of the first, so a later Enter cannot write there (task 82).
func TestPicker_SelectingAnotherEntityDropsTheEdit(t *testing.T) {
	f := startProjects(t, func(api *apiclient.Client) {
		if _, err := api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: "Line 1"}); err != nil {
			t.Fatalf("create project: %v", err)
		}
	})
	f.run()
	f.selectByName(servertest.MainProjectName)

	// A refusal keeps the field open on the entity it belongs to.
	f.p.startEdit(f.p.compoCtx, fieldName)
	f.p.value = "Line 1"
	f.p.commitEdit(f.p.compoCtx)
	f.run()
	if f.p.field != fieldName || f.p.editError == "" {
		t.Fatalf("the refused edit did not stay in its field: %q %q", f.p.field, f.p.editError)
	}

	// Picking another entity drops it rather than showing it there.
	line := f.selectByName("Line 1")
	if f.p.field != fieldNone || f.p.value != "" || f.p.editError != "" {
		t.Errorf("the edit of Main survived the selection: field %q, value %q, error %q",
			f.p.field, f.p.value, f.p.editError)
	}
	if f.p.SelectedID() != line.ID {
		t.Errorf("selected %v, want Line 1", f.p.SelectedID())
	}
	if got, err := f.api.GetProject(context.Background(), servertest.MainProject); err != nil || got.Name != servertest.MainProjectName {
		t.Errorf("the refused edit changed the project: %+v, %v", got, err)
	}
}

// TestPicker_EditSavesNameAndDescription: an edit of either field changes it,
// stays on the page and sends the whole pair (task 82).
func TestPicker_EditSavesNameAndDescription(t *testing.T) {
	f := startProjects(t, nil)
	f.run()

	f.selectByName(servertest.MainProjectName)
	f.p.startEdit(f.p.compoCtx, fieldName)
	f.p.value = "  Plant "
	f.p.commitEdit(f.p.compoCtx)
	f.run()

	f.p.startEdit(f.p.compoCtx, fieldDescription)
	f.p.value = "The plant line"
	f.p.commitEdit(f.p.compoCtx)
	f.run()

	if f.p.field != fieldNone || f.p.editError != "" {
		t.Fatalf("field %q: %q", f.p.field, f.p.editError)
	}
	got, err := f.api.GetProject(context.Background(), servertest.MainProject)
	if err != nil || got.Name != "Plant" || got.Description != "The plant line" {
		t.Fatalf("the updated project: %+v, %v", got, err)
	}
	if len(f.opened) != 0 {
		t.Errorf("an edit opened the editor: %v", f.opened)
	}
	if len(f.p.items) != 1 || f.p.items[0].Name != "Plant" {
		t.Errorf("listed %+v, want the new name", f.p.items)
	}
}

// TestPicker_UnchangedEditSendsNothing: a field left as it was asks the server
// for nothing (task 82).
func TestPicker_UnchangedEditSendsNothing(t *testing.T) {
	f := startProjects(t, nil)
	f.run()

	f.selectByName(servertest.MainProjectName)
	before, err := f.api.GetProject(context.Background(), servertest.MainProject)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	// The field opens on the value the entity holds, so leaving it alone is
	// no change at all.
	f.p.startEdit(f.p.compoCtx, fieldDescription)
	f.p.commitEdit(f.p.compoCtx)
	f.run()

	after, err := f.api.GetProject(context.Background(), servertest.MainProject)
	if err != nil {
		t.Fatalf("GetProject after the edit: %v", err)
	}
	if after.Version != before.Version {
		t.Errorf("an unchanged edit bumped the version: %d -> %d", before.Version, after.Version)
	}
	if f.p.field != fieldNone {
		t.Errorf("the field is still open: %q", f.p.field)
	}
}

// TestPicker_DeleteSelectsTheNext: deleting the shown entity shows the one
// that took its place, so the details are never empty while something is
// left (task 82).
func TestPicker_DeleteSelectsTheNext(t *testing.T) {
	var line uuid.UUID
	f := startProjects(t, func(api *apiclient.Client) {
		created, err := api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: "Line 1"})
		if err != nil {
			t.Fatalf("create project: %v", err)
		}
		line = created.ID
	})
	f.run()
	f.selectByName("Line 1")
	f.run()

	f.p.doDelete(f.p.compoCtx, line)
	f.run()

	if _, err := f.api.GetProject(context.Background(), line); err == nil {
		t.Error("the project is still on the server")
	}
	if f.p.SelectedID() != servertest.MainProject {
		t.Errorf("selected %v after the delete, want Main", f.p.SelectedID())
	}
	if len(f.p.items) != 1 || f.p.items[0].ID != servertest.MainProject {
		t.Errorf("listed %+v, want Main only", f.p.items)
	}
}

// TestPicker_ReloadsOnServerEvents: a project_created, a Save, a write of a
// Draft's content or a resync rereads the list; a library event does not
// (task 73).
func TestPicker_ReloadsOnServerEvents(t *testing.T) {
	f := startProjects(t, nil)
	f.run()
	if len(f.p.items) != 1 {
		t.Fatalf("listed %d projects, want Main", len(f.p.items))
	}

	f.create("Line 1", "")
	f.p.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, apiv0.Event{Type: apiv0.EventProjectCreated, ID: uuid.NewString()})
	f.run()
	if len(f.p.items) != 2 {
		t.Errorf("listed %d projects after project_created, want 2", len(f.p.items))
	}

	f.p.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, apiv0.Event{Type: apiv0.EventLibraryCreated, ID: uuid.NewString()})
	f.run()
	if len(f.p.items) != 2 {
		t.Errorf("a library event changed the project list: %d", len(f.p.items))
	}

	// A Save fixes a Draft, and a write of one makes the project unsaved: the
	// row's dot follows both (task 73).
	f.create("Line 2", "")
	f.p.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, apiv0.Event{Type: apiv0.EventCommitCreated, ID: uuid.NewString()})
	f.run()
	if len(f.p.items) != 3 {
		t.Errorf("listed %d projects after commit_created, want 3", len(f.p.items))
	}

	f.create("Line 3", "")
	f.p.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, apiv0.Event{Type: apiv0.EventTagCreated, ID: uuid.NewString()})
	f.run()
	if len(f.p.items) != 4 {
		t.Errorf("listed %d projects after tag_created, want 4", len(f.p.items))
	}

	f.create("Line 4", "")
	f.p.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, apiv0.Event{Type: eventlog.EventResync})
	f.run()
	if len(f.p.items) != 5 {
		t.Errorf("listed %d projects after a resync, want 5", len(f.p.items))
	}
}

// TestPicker_LibraryStates: the library rows carry the five states of
// LibraryResponse: never saved, unsaved changes, an unpublished Commit, a
// published one, and a legacy Release older than Commits (task 73; task 74).
func TestPicker_LibraryStates(t *testing.T) {
	f := startLibraries(t, nil)
	ctx := context.Background()
	created, err := f.api.CreateLibrary(ctx, apiv0.CreateLibraryRequest{Name: "Widgets", Description: "Shared widgets"})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	lib := f.api.Library(created.ID)
	reload := func() Item {
		t.Helper()
		f.p.load(f.p.compoCtx)
		f.run()
		if shown := f.p.Displayed(); len(shown) > 0 {
			return shown[0]
		}
		t.Fatalf("listed nothing")
		return Item{}
	}

	// A library nothing was written to is clean and never saved: no badge.
	if item := reload(); item.Name != "Widgets" || item.Description != "Shared widgets" ||
		item.State != StateNeverSaved || item.StateKind != "new" {
		t.Fatalf("the new library's row: %+v, want never saved", item)
	}
	if f.p.source.Title() != "Libraries" {
		t.Errorf("the page title: %q", f.p.source.Title())
	}

	// A write of the Draft is unsaved changes.
	state, err := lib.DraftState(ctx)
	if err != nil {
		t.Fatalf("DraftState: %v", err)
	}
	if _, err := lib.CreateWidgetType(ctx, apiv0.CreateWidgetTypeRequest{
		Name: "gauge", HtmlTemplate: "<div></div>", Script: "function update(){}",
		ScriptLanguage: "javascript", DefaultWidth: 100, DefaultHeight: 100, Version: state.Version,
	}); err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	if item := reload(); item.State != StateUnsavedChanges || item.StateKind != "unsaved" {
		t.Errorf("after a write: %+v, want unsaved changes", item)
	}

	// A Save fixes the Commit, nothing published yet.
	state, err = lib.DraftState(ctx)
	if err != nil {
		t.Fatalf("DraftState: %v", err)
	}
	commit, err := lib.SaveCommit(ctx, apiv0.CreateCommitRequest{Version: state.Version, Title: "First", Author: "Ada"})
	if err != nil {
		t.Fatalf("SaveCommit: %v", err)
	}
	if item := reload(); item.State != StateUnpublishedCommit || item.StateKind != "unpublished" {
		t.Errorf("after a Save: %+v, want an unpublished commit", item)
	}

	// A Release publishes it.
	if _, err := lib.Release(ctx, apiv0.CreateReleaseRequest{Commit: commit.Number, Author: "Ada"}); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if item := reload(); item.State != StatePublished || item.StateKind != "published" {
		t.Errorf("after a Release: %+v, want published", item)
	}

	// A Release older than Commits — a legacy one — is a library whose Draft
	// has no Commit and one Release.
	legacyID := uuid.New()
	if _, err := postgres.DB(t).ExecContext(ctx,
		`INSERT INTO engineering.libraries (id, organization_id, name, version)
		 SELECT $1, id, 'Legacy', 1 FROM organizations ORDER BY pk_id LIMIT 1`,
		legacyID,
	); err != nil {
		t.Fatalf("arrange the legacy library: %v", err)
	}
	if _, err := postgres.DB(t).ExecContext(ctx,
		`INSERT INTO engineering.library_releases (library_id, number) VALUES ($1, 1)`,
		legacyID,
	); err != nil {
		t.Fatalf("arrange the legacy release: %v", err)
	}
	f.p.load(f.p.compoCtx)
	f.run()
	shown := f.p.Displayed()
	if len(shown) != 2 || shown[0].Name != "Legacy" ||
		shown[0].State != StateLegacy || shown[0].StateKind != "legacy" {
		t.Errorf("the legacy library's row: %+v, want it first and legacy", shown)
	}
}

// TestPicker_StoredSelectionReported: the selection a previous visit kept is
// announced once the list is known — with its name — and one that is gone is
// reported as none (task 73).
func TestPicker_StoredSelectionReported(t *testing.T) {
	f := startProjects(t, nil)
	f.run()

	f.p.selected = servertest.MainProject
	f.p.load(f.p.compoCtx)
	f.run()
	if len(f.selected) == 0 || f.selected[len(f.selected)-1] != servertest.MainProject {
		t.Errorf("announced %v, want Main", f.selected)
	}
	if got := f.p.Name(servertest.MainProject); got != servertest.MainProjectName {
		t.Errorf("the name announced: %q", got)
	}

	f.selected = nil
	f.p.selected = uuid.New()
	f.p.load(f.p.compoCtx)
	f.run()
	if len(f.selected) != 1 || f.selected[0] != uuid.Nil {
		t.Errorf("announced %v, want none", f.selected)
	}
}

// TestPicker_ReloadReadsTheListAgain: Root asks a shown page to reread its
// list; before the page mounts it has nothing to read, and OnMount does it
// (task 73).
func TestPicker_ReloadReadsTheListAgain(t *testing.T) {
	f := startProjects(t, nil)
	f.run()

	unmounted := New(NewProjectSource(f.api), ProjectKey, nil, nil)
	unmounted.Reload(app.Context{}) // no context yet: must not read anything
	if unmounted.loaded || len(unmounted.items) != 0 {
		t.Errorf("a page not mounted read its list: %+v", unmounted)
	}

	f.create("Line 1", "")
	f.p.Reload(f.p.compoCtx)
	f.run()
	if len(f.p.items) != 2 {
		t.Errorf("listed %d projects after a reload, want 2", len(f.p.items))
	}
}

// TestPicker_NoSelectionLists: with no entity selected the page reports none
// and lists what the server has (Root shows it then).
func TestPicker_NoSelectionLists(t *testing.T) {
	f := startProjects(t, nil)
	f.run()
	if f.p.SelectedID() != uuid.Nil {
		t.Errorf("selected %v, want none", f.p.SelectedID())
	}
	if html := app.HTMLString(f.p.Render()); !strings.Contains(html, servertest.MainProjectName) {
		t.Errorf("the page does not list Main: %s", html)
	}
	// A failed request is not a panic: the page keeps saying it is loading.
	p := New(NewProjectSource(apiclient.New("http://127.0.0.1:1", http.DefaultClient)), ProjectKey, nil, nil)
	e := uitest.Load(t, mountOnHost{p})
	e.ConsumeAll()
	if html := app.HTMLString(p.Render()); !strings.Contains(html, "Projects") || !strings.Contains(html, "Loading projects…") {
		t.Errorf("an unreachable server: %s", html)
	}
}

// TestPicker_SelectingShowsTheDetailsOfOne: the details pane shows the entity
// the list reports, and the list marks exactly that row (task 82).
func TestPicker_SelectingShowsTheDetailsOfOne(t *testing.T) {
	f := startProjects(t, func(api *apiclient.Client) {
		if _, err := api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: "Alpha", Description: "A line"}); err != nil {
			t.Fatalf("create project: %v", err)
		}
	})
	f.run()

	alpha := f.selectByName("Alpha")
	if f.p.SelectedID() != alpha.ID {
		t.Errorf("selected %v, want Alpha", f.p.SelectedID())
	}
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-picker-selected="1"`) || !strings.Contains(html, "A line") {
		t.Errorf("the details do not follow the selection: %s", html)
	}
	if !slices.Contains(f.selected, alpha.ID) {
		t.Errorf("the selection was not reported: %v", f.selected)
	}
}
