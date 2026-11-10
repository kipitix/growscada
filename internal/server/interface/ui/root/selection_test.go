package root

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uitest"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// TestSelectionPage_TabOpensTheLastEntityOrTheList: a tab of the Library and
// Project modes shows the editor of the entity selected, and the page when
// there is none; the slim "← List" row shows the page, and the next tab
// opens the editor again (task 73).
func TestSelectionPage_TabOpensTheLastEntityOrTheList(t *testing.T) {
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

	r.showListPage(ModeProject)
	if !r.showList[ModeProject] {
		t.Error("the back row does not show the page")
	}
	// The entity selected stays announced to the event stream while the page
	// is shown, so the editor and the page follow the same one.
	if got, _ := r.barProject(ModeProject); got != main {
		t.Errorf("the page changed the Bar's project to %v", got)
	}

	r.SetMode(ModeProject)
	if r.showList[ModeProject] {
		t.Error("the tab does not open the last project again")
	}

	// Nothing selected: the tab shows the page, and the editor is not offered.
	r.selectEntity(ModeProject, uuid.Nil, "")
	if !r.showList[ModeProject] {
		t.Error("with no project the editor is shown")
	}
	r.SetMode(ModeProject)
	if !r.showList[ModeProject] {
		t.Error("with no project the tab hides the page")
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

// TestBackBar_ShowsTheListAndTheEntity: the slim row reads "← List · <name>"
// and names the page without one.
func TestBackBar_ShowsTheListAndTheEntity(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")
	r.modes = nil
	r.entityName[ModeProject] = "Main"

	if got := app.HTMLString(r.renderBackBar(ModeProject)); !strings.Contains(got, "← List · Main") {
		t.Errorf("the back row: %s", got)
	}
	r.entityName[ModeProject] = ""
	if got := app.HTMLString(r.renderBackBar(ModeProject)); !strings.Contains(got, "← List") || strings.Contains(got, " · ") {
		t.Errorf("the back row without a name: %s", got)
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
