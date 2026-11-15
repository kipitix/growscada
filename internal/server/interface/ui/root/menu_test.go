package root

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/picker"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// menuSession mounts a Root at url with the LocalStorage a previous session
// left, as mountRoot does for the address it is given.
func menuSession(t *testing.T, url string, stored map[string]string) *rootSession {
	t.Helper()
	engine := app.NewTestEngine()
	probe := &storageProbe{}
	if err := engine.Load(probe); err != nil {
		t.Fatalf("load the storage probe: %v", err)
	}
	engine.ConsumeAll()
	for key, value := range stored {
		if err := probe.ctx.LocalStorage().Set(key, value); err != nil {
			t.Fatalf("store %q: %v", key, err)
		}
	}
	r := NewRoot(url)
	if err := engine.Load(mountOnHost{r}); err != nil {
		t.Fatalf("load the root: %v", err)
	}
	engine.ConsumeAll()
	return &rootSession{Root: r, engine: engine, probe: probe}
}

// TestMenu_OpenExpandsTheModeShown: the burger opens the menu on the mode shown,
// and the menu is what the header holds, not a mode of its own (task 89).
func TestMenu_OpenExpandsTheModeShown(t *testing.T) {
	s := menuSession(t, "http://127.0.0.1:0", nil)
	s.SetMode(ModeProject)
	s.settle()

	s.SetMode(ModeHistory) // no projects: nothing selected, the mode shown
	s.settle()
	s.compoCtx.Dispatch(func(ctx app.Context) { s.openMenu(ctx) })
	s.settle()

	if !s.menuOpen || s.menuExpanded != ModeHistory {
		t.Fatalf("menu open %t on %q, want open on the mode shown", s.menuOpen, s.menuExpanded)
	}
	s.compoCtx.Dispatch(func(ctx app.Context) { s.closeMenu() })
	s.settle()
	if s.menuOpen {
		t.Error("the menu stays open after closing it")
	}
}

// TestMenu_ModeRowOnlyExpands: a row of a mode expands or folds the entities of
// that mode and never switches the mode shown (task 89).
func TestMenu_ModeRowOnlyExpands(t *testing.T) {
	s := menuSession(t, "http://127.0.0.1:0", nil)
	s.SetMode(ModeLibrary)
	s.settle()
	s.compoCtx.Dispatch(func(ctx app.Context) { s.openMenu(ctx) })
	s.settle()

	s.compoCtx.Dispatch(func(ctx app.Context) { s.expandMode(ctx, ModeProject) })
	s.settle()
	if s.menuExpanded != ModeProject {
		t.Errorf("expanded %q, want the project", s.menuExpanded)
	}
	if s.currentMode != ModeLibrary {
		t.Errorf("the mode row switched the mode to %q", s.currentMode)
	}

	s.compoCtx.Dispatch(func(ctx app.Context) { s.expandMode(ctx, ModeProject) })
	s.settle()
	if s.menuExpanded != ModeUnknown {
		t.Errorf("the row expanded again: %q, want it folded", s.menuExpanded)
	}
}

// TestMenu_ChooseEntityOpensItsEditor: a library or a project chosen in the
// menu opens its editor at once, under the name the menu read, switches the
// mode and closes the menu (task 89).
func TestMenu_ChooseEntityOpensItsEditor(t *testing.T) {
	for _, mode := range []Mode{ModeLibrary, ModeProject} {
		t.Run(string(mode), func(t *testing.T) {
			s := menuSession(t, "http://127.0.0.1:0", nil)
			id := uuid.New()
			item := picker.Item{ID: id, Name: "Greenhouse"}

			s.compoCtx.Dispatch(func(ctx app.Context) {
				s.openMenu(ctx)
				s.chooseEntity(ctx, mode, item)
			})
			s.settle()

			if s.menuOpen {
				t.Error("the menu stays open after a choice")
			}
			if s.currentMode != mode {
				t.Errorf("the mode shown is %q, want %q", s.currentMode, mode)
			}
			if s.showList[mode] {
				t.Error("the choice shows the selection page, want the editor")
			}
			if got := s.selected[mode]; got != id {
				t.Errorf("the mode shows %v, want %v", got, id)
			}
			if got := s.entityName[mode]; got != "Greenhouse" {
				t.Errorf("the entity is named %q, want the name the menu read", got)
			}
			if got := s.screen(t, mode); got != screenEditor {
				t.Errorf("the screen stored is %q, want the editor", got)
			}
			var shown string
			if err := s.probe.ctx.LocalStorage().Get("root:mode", &shown); err != nil || shown != string(mode) {
				t.Errorf("the mode stored is %q (%v), want %q", shown, err, mode)
			}
		})
	}
}

// TestMenu_ChoosePageShowsTheList: the way to the selection page of a mode
// switches to it and shows the page, where the entities are created (task 89).
func TestMenu_ChoosePageShowsTheList(t *testing.T) {
	s := menuSession(t, "http://127.0.0.1:0", nil)

	s.compoCtx.Dispatch(func(ctx app.Context) {
		s.openMenu(ctx)
		s.choosePage(ctx, ModeProject)
	})
	s.settle()

	if s.menuOpen {
		t.Error("the menu stays open after the choice")
	}
	if s.currentMode != ModeProject || !s.showList[ModeProject] {
		t.Errorf("mode %q, page shown %t; want the Project page", s.currentMode, s.showList[ModeProject])
	}
	if got := s.screen(t, ModeProject); got != screenList {
		t.Errorf("the screen stored is %q, want the page", got)
	}
}

// TestMenu_ChooseProjectForOperationAndHistory: a row of a project under
// Operation or History takes it, keeps it for the next reload and switches to
// the mode (task 89).
func TestMenu_ChooseProjectForOperationAndHistory(t *testing.T) {
	for _, mode := range []Mode{ModeOperation, ModeHistory} {
		t.Run(string(mode), func(t *testing.T) {
			s := menuSession(t, "http://127.0.0.1:0", nil)
			id := uuid.New()

			s.compoCtx.Dispatch(func(ctx app.Context) {
				s.openMenu(ctx)
				s.chooseEntity(ctx, mode, picker.Item{ID: id, Name: "Greenhouse"})
			})
			s.settle()

			if s.menuOpen {
				t.Error("the menu stays open after the choice")
			}
			if s.currentMode != mode {
				t.Errorf("the mode shown is %q, want %q", s.currentMode, mode)
			}
			if got := s.selected[mode]; got != id {
				t.Errorf("the mode shows %v, want %v", got, id)
			}
			if got := s.entityName[mode]; got != "Greenhouse" {
				t.Errorf("the project is named %q", got)
			}
			if got := uiutil.LoadID(s.probe.ctx, projectKey(mode)); got != id {
				t.Errorf("the project kept is %v, want %v", got, id)
			}
			if mode == ModeOperation {
				if got := s.bar.Project(); got != id {
					t.Errorf("the Bar follows %v, want %v", got, id)
				}
			}
		})
	}
}

// TestNavigation_NamesTheModeAndTheEntity: the line beside the burger says the
// mode, and the entity it shows after a middle dot; with no entity it says the
// mode alone (task 89).
func TestNavigation_NamesTheModeAndTheEntity(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")

	r.currentMode = ModeLibrary
	if got := r.navigationText(); got != "Library" {
		t.Errorf("with no library: %q, want the mode alone", got)
	}
	r.entityName[ModeLibrary] = "Greenhouse"
	if got := r.navigationText(); got != "Library \u00b7 Greenhouse" {
		t.Errorf("with a library: %q, want the mode and its name around a middle dot", got)
	}
	r.showList[ModeLibrary] = true
	if got := r.navigationText(); got != "Library" {
		t.Errorf("on the selection page: %q, want the mode alone", got)
	}
	r.showList[ModeLibrary] = false

	r.currentMode = ModeOperation
	r.entityName[ModeOperation] = "Greenhouse"
	if got := r.navigationText(); got != "Operation \u00b7 Greenhouse" {
		t.Errorf("operation: %q", got)
	}
	if html := app.HTMLString(r.renderNavigation()); !strings.Contains(html, "data-navigation") ||
		!strings.Contains(html, "Operation \u00b7 Greenhouse") {
		t.Errorf("the navigation line lacks its text: %s", html)
	}
}

// TestMenu_ListsEntitiesOfTheExpandedModeOnly: the open menu lists the modes,
// the expanded one with its entities and the way to its page; the modes folded
// list nothing, and a mode without entities says which of the two it is — still
// reading them, or none on the server (task 89).
func TestMenu_ListsEntitiesOfTheExpandedModeOnly(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")
	r.menuOpen, r.menuExpanded = true, ModeLibrary
	r.menuLibraries = []picker.Item{
		{ID: uuid.New(), Name: "Zulu"},
		{ID: uuid.New(), Name: "Alpha"},
	}
	r.menuLibrariesLoaded = true

	html := app.HTMLString(r.renderMenuPanel())
	for _, want := range []string{`id="gs-main-menu"`, `data-menu-mode="library"`, `data-menu-mode="project"`,
		`data-menu-mode="operation"`, `data-menu-mode="history"`, `data-menu-choose="library"`,
		`data-menu-entity`} {
		if !strings.Contains(html, want) {
			t.Errorf("the panel lacks %s: %s", want, html)
		}
	}
	if strings.Contains(html, `data-menu-choose="project"`) {
		t.Error("the folded project mode lists its page")
	}
	// The entities come in the order the person placed, then by name.
	if i, j := strings.Index(html, "Alpha"), strings.Index(html, "Zulu"); i < 0 || j < 0 || i > j {
		t.Errorf("the libraries are out of order: %s", html)
	}

	// Read and empty: the hint says the server has none, not that it is
	// loading. Not read yet: it says it is loading.
	r.menuLibraries, r.menuLibrariesLoaded, r.menuExpanded = nil, true, ModeProject
	r.menuProjectsLoaded = true
	if html := app.HTMLString(r.renderMenuPanel()); !strings.Contains(html, "No projects yet") {
		t.Errorf("an empty list does not say so: %s", html)
	}
	r.menuProjectsLoaded = false
	if html := app.HTMLString(r.renderMenuPanel()); !strings.Contains(html, "Loading projects…") {
		t.Errorf("a list being read does not say so: %s", html)
	}
}

// TestMenu_OperationWithoutProjectsIsDisabled: with no project on the server
// Operation and History have nothing to enter, and their rows say why (task
// 89).
func TestMenu_OperationWithoutProjectsIsDisabled(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")
	r.menuOpen, r.menuExpanded = true, ModeProject
	r.menuProjectsLoaded = true // the server has none

	for _, mode := range []Mode{ModeOperation, ModeHistory} {
		if r.modeEnterable(mode) {
			t.Errorf("%q is offered without a project", mode)
		}
	}
	html := app.HTMLString(r.renderMenuPanel())
	if got := strings.Count(html, "disabled"); got != 2 {
		t.Errorf("%d rows are disabled, want the two modes without a project: %s", got, html)
	}
	if !strings.Contains(html, "No projects yet") {
		t.Errorf("the rows do not say why: %s", html)
	}
}

// TestMenu_LibrariesKeepThePersonsOrder: the menu lists the libraries as the
// selection page does — the IDs the person placed first, then the rest by name
// (ADR 0024).
func TestMenu_LibrariesKeepThePersonsOrder(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")
	first, second := uuid.New(), uuid.New()
	r.menuLibraries = []picker.Item{
		{ID: first, Name: "Alpha"},
		{ID: second, Name: "Zulu"},
	}
	r.menuLibraryOrder = picker.Order{IDs: []uuid.UUID{second}}

	items := r.menuItems(ModeLibrary)
	if len(items) != 2 || items[0].ID != second || items[1].ID != first {
		t.Errorf("the libraries come as %v, want %v first", items, second)
	}
}

// TestRestoreProjects_ReadsTheNamesAndDropsAProjectGone: a reload names the
// project Operation and History show, and leaves the mode without one when the
// server no longer has it (task 89).
func TestRestoreProjects_ReadsTheNamesAndDropsAProjectGone(t *testing.T) {
	kept, gone := uuid.New(), uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projects":[{"id":"` + kept.String() + `","name":"Greenhouse","version":1}]}`))
	}))
	defer srv.Close()

	s := menuSession(t, srv.URL, map[string]string{
		projectKey(ModeOperation): kept.String(),
		projectKey(ModeHistory):   gone.String(),
	})
	s.settle()

	if got := s.entityName[ModeOperation]; got != "Greenhouse" {
		t.Errorf("operation names its project %q, want Greenhouse", got)
	}
	if got := s.selected[ModeHistory]; got != uuid.Nil {
		t.Errorf("history keeps a project the server does not have: %v", got)
	}
	if got := uiutil.LoadID(s.probe.ctx, projectKey(ModeHistory)); got != uuid.Nil {
		t.Errorf("the key of the project gone is kept: %v", got)
	}
}

// TestProjectEvents_RenameAndDeletion: a project renamed elsewhere renames the
// line of the mode showing it; a project deleted elsewhere leaves the mode
// without one, never with a stale name (task 89).
func TestProjectEvents_RenameAndDeletion(t *testing.T) {
	id := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"` + id.String() + `","name":"Renamed","version":2}`))
	}))
	defer srv.Close()

	s := menuSession(t, srv.URL, map[string]string{projectKey(ModeOperation): id.String()})
	s.selected[ModeOperation] = id
	s.entityName[ModeOperation] = "Before"
	s.settle()

	s.compoCtx.Dispatch(func(ctx app.Context) {
		s.handleProjectEvents(ctx, apiv0.Event{Type: apiv0.EventProjectUpdated, ID: id.String()})
	})
	s.settle()
	if got := s.entityName[ModeOperation]; got != "Renamed" {
		t.Errorf("the rename left the name %q", got)
	}

	s.compoCtx.Dispatch(func(ctx app.Context) {
		s.handleProjectEvents(ctx, apiv0.Event{Type: apiv0.EventProjectDeleted, ID: id.String()})
	})
	s.settle()
	if got := s.selected[ModeOperation]; got != uuid.Nil {
		t.Errorf("the deleted project is still selected: %v", got)
	}
	if got := s.entityName[ModeOperation]; got != "" {
		t.Errorf("the deleted project is still named %q", got)
	}
	if got := uiutil.LoadID(s.probe.ctx, projectKey(ModeOperation)); got != uuid.Nil {
		t.Errorf("the deleted project is still kept: %v", got)
	}
}

// TestHeader_CarriesTheMenuAndNoTabs: the header is the burger, the navigation
// line and the shared actions; it carries no tab list any more (task 89).
func TestHeader_CarriesTheMenuAndNoTabs(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")
	r.currentMode = ModeProject
	r.entityName[ModeProject] = "Greenhouse"

	html := app.HTMLString(r.renderHeader())
	for _, want := range []string{"data-header", "gs-main-menu-button", `aria-expanded="false"`,
		"aria-haspopup", "gs-main-navigation", "Project \u00b7 Greenhouse", "data-labels-open"} {
		if !strings.Contains(html, want) {
			t.Errorf("the header lacks %s: %s", want, html)
		}
	}
	if strings.Contains(html, "tablist") || strings.Contains(html, `role="tab"`) {
		t.Errorf("the header still carries tabs: %s", html)
	}
}
