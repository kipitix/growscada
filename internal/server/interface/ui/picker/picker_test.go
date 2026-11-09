package picker

import (
	"context"
	"net/http"
	"os"
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
	names := make([]string, len(f.p.items))
	for i, item := range f.p.items {
		names[i] = item.Name
	}
	return names
}

// TestPicker_ListsSortedWithDescriptionAndState: the page reads the live
// entities on mount, sorts them by name and shows the description and the
// unsaved-changes badge (task 73).
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
	html := app.HTMLString(f.p.Render())
	for _, want := range []string{`data-picker-row`, "The last line", "Unsaved changes", "No description", "Projects"} {
		if !strings.Contains(html, want) {
			t.Errorf("the page does not show %q: %s", want, html)
		}
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

// TestPicker_CreateOpensTheNewEntity: Create validates nothing on its own,
// the server is the only validator, and a created entity opens at once
// (task 73).
func TestPicker_CreateOpensTheNewEntity(t *testing.T) {
	f := startProjects(t, nil)
	f.run()

	f.p.openDialog(f.p.compoCtx, uuid.Nil)
	f.p.dialogName = "Line 1"
	f.p.dialogDescription = "  A new line  "
	f.p.submitDialog(f.p.compoCtx)
	f.run()

	if f.p.dialog {
		t.Errorf("the dialog is still open: %q", f.p.dialogError)
	}
	created, ok, err := f.api.FindProjectByName(context.Background(), "Line 1")
	if err != nil || !ok {
		t.Fatalf("Line 1 on the server: %v, %v", ok, err)
	}
	if created.Description != "A new line" {
		t.Errorf("the description stored: %q", created.Description)
	}
	if len(f.opened) != 1 || f.opened[0] != created.ID {
		t.Errorf("opened %v, want the created project", f.opened)
	}
	if len(f.p.items) != 2 {
		t.Errorf("listed %d projects after the create, want 2", len(f.p.items))
	}
}

// TestPicker_EditErrorStaysInTheDialog: a refusal of the server — here a name
// taken — is shown inside the dialog, which keeps the input (task 73).
func TestPicker_EditErrorStaysInTheDialog(t *testing.T) {
	f := startProjects(t, func(api *apiclient.Client) {
		if _, err := api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: "Line 1"}); err != nil {
			t.Fatalf("create project: %v", err)
		}
	})
	f.run()

	f.p.openDialog(f.p.compoCtx, servertest.MainProject)
	f.p.dialogName = "Line 1"
	f.p.dialogDescription = "Kept"
	f.p.submitDialog(f.p.compoCtx)
	f.run()

	if !f.p.dialog || f.p.dialogError == "" || !strings.Contains(f.p.dialogError, "taken") {
		t.Fatalf("dialog %t, error %q; want the refusal inside the dialog", f.p.dialog, f.p.dialogError)
	}
	if f.p.dialogName != "Line 1" || f.p.dialogDescription != "Kept" {
		t.Errorf("the input was lost: %q %q", f.p.dialogName, f.p.dialogDescription)
	}
	html := app.HTMLString(f.p.Render())
	if !strings.Contains(html, `data-picker-error`) || !strings.Contains(html, "taken") || !strings.Contains(html, "Kept") {
		t.Errorf("the dialog does not show the refusal and the input: %s", html)
	}
	if f.p.EditingID() != servertest.MainProject {
		t.Errorf("editing %v, want Main", f.p.EditingID())
	}
}

// TestPicker_EditSavesNameAndDescription: an edit changes both and stays on
// the list.
func TestPicker_EditSavesNameAndDescription(t *testing.T) {
	f := startProjects(t, nil)
	f.run()

	f.p.openDialog(f.p.compoCtx, servertest.MainProject)
	f.p.dialogName = "Plant"
	f.p.dialogDescription = "The plant line"
	f.p.submitDialog(f.p.compoCtx)
	f.run()

	if f.p.dialog || f.p.dialogError != "" {
		t.Fatalf("dialog %t: %q", f.p.dialog, f.p.dialogError)
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

// TestPicker_DeleteSelectedLeavesNone: deleting the shown entity selects none
// — the mode shows the page then — and deleting another one keeps it.
func TestPicker_DeleteSelectedLeavesNone(t *testing.T) {
	var line uuid.UUID
	f := startProjects(t, func(api *apiclient.Client) {
		created, err := api.CreateProject(context.Background(), apiv0.CreateProjectRequest{Name: "Line 1"})
		if err != nil {
			t.Fatalf("create project: %v", err)
		}
		line = created.ID
	})
	f.run()
	f.p.open(f.p.compoCtx, line)
	f.run()

	f.p.doDelete(f.p.compoCtx, line)
	f.run()

	if _, err := f.api.GetProject(context.Background(), line); err == nil {
		t.Error("the project is still on the server")
	}
	if f.p.SelectedID() != uuid.Nil || len(f.selected) == 0 || f.selected[len(f.selected)-1] != uuid.Nil {
		t.Errorf("selected %v, want none: %v", f.p.SelectedID(), f.selected)
	}
	if len(f.p.items) != 1 || f.p.items[0].ID != servertest.MainProject {
		t.Errorf("listed %+v, want Main only", f.p.items)
	}
}

// TestPicker_ReloadsOnServerEvents: a project_created or a resync rereads the
// list; a library event does not (task 73).
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

	f.create("Line 2", "")
	f.p.compoCtx.NewActionWithValue(eventlog.ActionServerEvent, apiv0.Event{Type: eventlog.EventResync})
	f.run()
	if len(f.p.items) != 3 {
		t.Errorf("listed %d projects after a resync, want 3", len(f.p.items))
	}
}

// TestPicker_LibraryStates: the library rows carry the Commit and Release
// state of LibraryResponse (task 73).
func TestPicker_LibraryStates(t *testing.T) {
	f := start(t, NewLibrarySource, LibraryKey, func(api *apiclient.Client) {
		if _, err := api.CreateLibrary(context.Background(), apiv0.CreateLibraryRequest{Name: "Widgets", Description: "Shared widgets"}); err != nil {
			t.Fatalf("create library: %v", err)
		}
	})
	f.run()

	if len(f.p.items) != 1 {
		t.Fatalf("listed %+v", f.p.items)
	}
	item := f.p.items[0]
	if item.Name != "Widgets" || item.Description != "Shared widgets" || item.State != StateUnsavedChanges {
		t.Errorf("the library row: %+v", item)
	}
	if f.p.source.Title() != "Libraries" {
		t.Errorf("the page title: %q", f.p.source.Title())
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
