package root

import (
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uitest"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// storageProbe holds the engine's LocalStorage: mounted before a Root, it lets
// a test set the keys a previous session left and read the ones this one
// leaves — what a reload of the page finds.
type storageProbe struct {
	app.Compo
	ctx app.Context
}

func (p *storageProbe) OnPreRender(ctx app.Context) { p.ctx = ctx }
func (p *storageProbe) Render() app.UI              { return app.Div() }

// rootSession is a Root mounted in a test engine: settle runs what its mounts
// and commands started, and screen reads the screen stored for a mode.
type rootSession struct {
	*Root
	engine app.TestEngine
	probe  *storageProbe
}

// mountRoot mounts a Root in a fresh test engine whose LocalStorage holds
// stored, the keys a previous session left. arrange, when given, gets the Root
// before it mounts, to put its parts in place.
func mountRoot(t *testing.T, stored map[string]string, arrange ...func(*Root)) *rootSession {
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
	r := NewRoot("http://127.0.0.1:0")
	for _, put := range arrange {
		put(r)
	}
	if err := engine.Load(mountOnHost{r}); err != nil {
		t.Fatalf("load the root: %v", err)
	}
	engine.ConsumeAll()
	return &rootSession{Root: r, engine: engine, probe: probe}
}

// settle runs what the mounts and the commands started.
func (s *rootSession) settle() { s.engine.ConsumeAll() }

// screen reads the screen stored for mode, "" when none is.
func (s *rootSession) screen(t *testing.T, mode Mode) string {
	t.Helper()
	var screen string
	if err := s.probe.ctx.LocalStorage().Get(screenKey(mode), &screen); err != nil {
		t.Fatalf("read the screen of %q: %v", mode, err)
	}
	return screen
}

// TestSelectionPage_TabKeepsTheScreen: a tab of the Library and Project modes
// opens the screen the person left in it — the editor of the entity shown or
// the selection page — so a walk through another tab and back, or a second
// click on the tab shown, returns to the same screen (task 85_1). A mode with
// nothing selected shows the page.
func TestSelectionPage_TabKeepsTheScreen(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")
	main := uuid.New()

	r.SetMode(ModeProject)
	if !r.showList[ModeProject] {
		t.Error("a tab with no project shows the editor")
	}

	r.selectEntity(ModeProject, main, "Main")
	r.openEntity(ModeProject, main)
	if r.showList[ModeProject] {
		t.Error("opening a project does not show the editor")
	}
	if got, _ := r.barProject(ModeProject); got != main {
		t.Errorf("the Bar follows %v, want %v", got, main)
	}

	// Another tab and back: the editor is what the Project tab was left on.
	r.SetMode(ModeLibrary)
	r.SetMode(ModeProject)
	if r.showList[ModeProject] {
		t.Error("a walk through another tab reset the project to its page")
	}

	r.showListPage(ModeProject)
	if !r.showList[ModeProject] {
		t.Error("the back row does not show the page")
	}
	// The entity selected stays announced to the event stream while the page
	// is shown, so the editor and the page follow the same one.
	if got, _ := r.barProject(ModeProject); got != main {
		t.Errorf("the page changed the Bar's project to %v", got)
	}

	// Clicking the tab shown again keeps the page it was left on.
	r.SetMode(ModeProject)
	if !r.showList[ModeProject] {
		t.Error("the tab reset the page to the editor")
	}

	// Nothing selected: the page, whichever screen the tab was left on.
	r.openEntity(ModeProject, main)
	r.selectEntity(ModeProject, uuid.Nil, "")
	if !r.showList[ModeProject] {
		t.Error("with no project the editor is shown")
	}
	r.SetMode(ModeProject)
	if !r.showList[ModeProject] {
		t.Error("with no project the tab hides the page")
	}
}

// TestSelectionPage_ScreenSurvivesAReload: the screen of a mode is stored in
// LocalStorage — library:screen and project:screen — so a reload of the tab
// opens the page or the editor the person left. A stored editor without an
// entity selected is the page, and no stored screen follows the selection as
// task 73 had it (task 85_1).
func TestSelectionPage_ScreenSurvivesAReload(t *testing.T) {
	lib, main := uuid.New(), uuid.New()
	cases := []struct {
		name    string
		storage map[string]string
		mode    Mode
		list    bool
	}{
		{"library: the page left shown", map[string]string{
			"library:selectedLibrary": lib.String(),
			"library:screen":          screenList,
		}, ModeLibrary, true},
		{"library: the editor left shown", map[string]string{
			"library:selectedLibrary": lib.String(),
			"library:screen":          screenEditor,
		}, ModeLibrary, false},
		{"library: no screen stored is the editor of the library", map[string]string{
			"library:selectedLibrary": lib.String(),
		}, ModeLibrary, false},
		{"library: no screen stored and no library is the page", map[string]string{},
			ModeLibrary, true},
		{"library: the editor stored without a library is the page", map[string]string{
			"library:screen": screenEditor,
		}, ModeLibrary, true},
		{"project: the page left shown", map[string]string{
			"root:mode":         string(ModeProject),
			"project:projectID": main.String(),
			"project:screen":    screenList,
		}, ModeProject, true},
		{"project: the editor left shown", map[string]string{
			"root:mode":         string(ModeProject),
			"project:projectID": main.String(),
			"project:screen":    screenEditor,
		}, ModeProject, false},
		{"project: no screen stored and no project is the page", map[string]string{
			"root:mode": string(ModeProject),
		}, ModeProject, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := mountRoot(t, c.storage)
			if s.currentMode != c.mode {
				t.Fatalf("the tab opened %q, want %q", s.currentMode, c.mode)
			}
			if got := s.showList[c.mode]; got != c.list {
				t.Errorf("page shown: %t, want %t", got, c.list)
			}
		})
	}
}

// TestSelectionPage_ScreenIsStored: opening an entity stores its editor, the
// way back and an entity found gone store the page, so the next reload opens
// what was left (task 85_1).
func TestSelectionPage_ScreenIsStored(t *testing.T) {
	for _, mode := range []Mode{ModeLibrary, ModeProject} {
		t.Run(string(mode), func(t *testing.T) {
			s := mountRoot(t, nil)
			id := uuid.New()

			s.SetMode(mode)
			s.selectEntity(mode, id, "Entity")
			s.openEntity(mode, id)
			if got := s.screen(t, mode); got != screenEditor {
				t.Errorf("the screen after opening an entity: %q, want %q", got, screenEditor)
			}

			s.showListPage(mode)
			if got := s.screen(t, mode); got != screenList {
				t.Errorf("the screen after the way back: %q, want %q", got, screenList)
			}

			// The entity of the editor found gone: the page, and stored.
			s.openEntity(mode, id)
			s.entityGone(mode, id)
			if got := s.screen(t, mode); got != screenList {
				t.Errorf("the screen after an entity found gone: %q, want %q", got, screenList)
			}
			if !s.showList[mode] {
				t.Error("an entity found gone does not show the page")
			}
		})
	}
}

// countingPage stands in for a selection page and counts the times Root asks
// it to reread its list (a real picker is not mounted on the host, so its own
// reads are not observable — see uitest).
type countingPage struct {
	app.Compo
	reloads *atomic.Int32
}

func (p countingPage) Reload(app.Context)          { p.reloads.Add(1) }
func (p countingPage) Show(app.Context, uuid.UUID) {}
func (p countingPage) Render() app.UI              { return app.Div() }

// TestSelectionPage_ThePageIsAskedToRereadWhenShown: the page rereads its list
// when the mode opens on it — the screen restored from LocalStorage included —
// and each time it is shown again, once per showing; the tab clicked again and
// the editor ask for nothing (task 73, task 85_1).
func TestSelectionPage_ThePageIsAskedToRereadWhenShown(t *testing.T) {
	var reloads atomic.Int32
	s := mountRoot(t, map[string]string{"library:screen": screenList}, func(r *Root) {
		r.modes[0].picker = &countingPage{reloads: &reloads}
	})

	// The first opening shows the restored page: it rereads its list.
	if got := reloads.Load(); got != 1 {
		t.Fatalf("the page asked to reread %d times by the first opening, want 1", got)
	}

	// A second click on the tab shown keeps the screen and asks for nothing.
	s.SetMode(ModeLibrary)
	s.settle()
	if got := reloads.Load(); got != 1 {
		t.Errorf("the tab clicked again asked the page %d times, want 1", got)
	}

	// The editor, then another tab: neither asks the page for anything.
	s.openEntity(ModeLibrary, uuid.New())
	s.SetMode(ModeProject)
	s.settle()
	s.SetMode(ModeLibrary)
	s.settle()
	if got := reloads.Load(); got != 1 {
		t.Errorf("the editor asked the page %d times, want 1", got)
	}

	// The way back shows the page and it rereads its list, once.
	s.showListPage(ModeLibrary)
	s.settle()
	if got := reloads.Load(); got != 2 {
		t.Errorf("the page shown again asked to reread %d times, want 2", got)
	}
	if !s.showList[ModeLibrary] {
		t.Error("the way back does not show the page")
	}
}

// TestSelectionPage_LibraryDoesNotFollowTheBar: the Library mode has no
// project of its own, so its selection leaves the event stream as it is.
func TestSelectionPage_LibraryDoesNotFollowTheBar(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")
	main, lib := uuid.New(), uuid.New()

	r.SetMode(ModeProject)
	r.selectEntity(ModeProject, main, "Main")
	r.SetMode(ModeLibrary)
	r.selectEntity(ModeLibrary, lib, "Widgets")
	r.SetMode(ModeLibrary)

	if got := r.bar.Project(); got != main {
		t.Errorf("the Bar follows %v, want the project %v", got, main)
	}
	if got := r.entityName[ModeLibrary]; got != "Widgets" {
		t.Errorf("the library's name for the back row: %q", got)
	}
}

// TestPanelBack_ShowsTheListAndTheEntity: the panel of the editor carries the
// way back to the selection page and the name of the entity shown (task 84):
// Root wires both from what the selection announced, and the way back shows
// the page.
func TestPanelBack_ShowsTheListAndTheEntity(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")
	main := uuid.New()

	r.selectEntity(ModeProject, main, "Main")
	if got := r.project.BackName(); got != "Main" {
		t.Errorf("the name of the panel: %q", got)
	}
	r.selectEntity(ModeLibrary, main, "Widgets")
	if got := r.library.BackName(); got != "Widgets" {
		t.Errorf("the name of the library's panel: %q", got)
	}

	r.openEntity(ModeProject, main)
	if r.showList[ModeProject] {
		t.Fatal("opening a project does not show the editor")
	}
	r.project.OnBack()
	if !r.showList[ModeProject] {
		t.Error("the way back does not show the selection page")
	}
}

// TestSelectionPage_GoneEntityReturnsToTheList: the action a read fires for the
// entity shown found gone — ActionLibraryGone for a Library, ActionProjectGone
// for a Project — leaves it and shows the selection page, whose list is reread;
// the deleted entity is not the one announced any more, the back row names
// none, and another one opens from the page as before (task 76).
func TestSelectionPage_GoneEntityReturnsToTheList(t *testing.T) {
	modes := []struct {
		mode   Mode
		action func(app.Context, uuid.UUID)
	}{
		{ModeLibrary, func(ctx app.Context, id uuid.UUID) { ctx.NewActionWithValue(uiutil.ActionLibraryGone, id) }},
		{ModeProject, func(ctx app.Context, id uuid.UUID) { ctx.NewActionWithValue(uiutil.ActionProjectGone, id) }},
	}
	for _, m := range modes {
		t.Run(string(m.mode), func(t *testing.T) {
			r := NewRoot("http://127.0.0.1:0")
			e := uitest.Load(t, mountOnHost{r})
			gone, kept := uuid.New(), uuid.New()

			r.SetMode(m.mode)
			r.selectEntity(m.mode, gone, "Gone")
			r.openEntity(m.mode, gone)
			e.ConsumeAll()
			if r.showList[m.mode] {
				t.Fatal("opening the entity does not show its editor")
			}

			// A read of it found it gone: the action names it.
			r.compoCtx.Dispatch(func(ctx app.Context) { m.action(ctx, gone) })
			e.ConsumeAll()

			if !r.showList[m.mode] || r.selected[m.mode] != uuid.Nil {
				t.Fatalf("an entity found gone: selected %v, page shown %t; want the page with none",
					r.selected[m.mode], r.showList[m.mode])
			}
			if name := r.entityName[m.mode]; name != "" {
				t.Errorf("the back row still names %q", name)
			}

			// Another one opens from the page, and a delayed 404 of the one
			// left changes nothing.
			r.selectEntity(m.mode, kept, "Kept")
			r.openEntity(m.mode, kept)
			if r.showList[m.mode] || r.selected[m.mode] != kept {
				t.Fatalf("the other entity: selected %v, page shown %t; want its editor",
					r.selected[m.mode], r.showList[m.mode])
			}
			r.entityGone(m.mode, gone)
			if r.showList[m.mode] || r.selected[m.mode] != kept {
				t.Errorf("a delayed 404 of an entity left selected %v", r.selected[m.mode])
			}
		})
	}
}
