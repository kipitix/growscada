package root

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/misc"
	"github.com/kipitix/growscada/internal/server/interface/ui/picker"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The main menu (task 89): the mode shown and the entity it shows are chosen
// in the menu behind the burger instead of the tabs the header used to carry.
// A row of a mode expands the entities of that mode; choosing one switches the
// mode and closes the menu. The navigation line beside the burger is a chain of
// buttons — the mode, the entity and the saved states when they are shown —
// each going one step up (task 91).
const (
	// menuButtonID and menuPanelID tie the burger to the panel it shows, for
	// aria-controls and for the focus the menu gives back.
	menuButtonID = "gs-main-menu-button"
	menuPanelID  = "gs-main-menu"
	// navigationID is the navigation line beside the burger.
	navigationID = "gs-main-navigation"
	// middleDot separates the steps of the navigation line: the dot sits at the
	// middle of the line's height.
	middleDot = " · "
	// menuPanelWidth is the width of the open menu on a screen with room for
	// it; on a phone it takes the whole width.
	menuPanelWidth = "340px"
	// menuScrollDelay waits for the panel the opening rendered to reach the
	// DOM before the entity shown is scrolled into view.
	menuScrollDelay = 50 * time.Millisecond
)

// modeLabels names the modes in the menu and in the navigation line.
var modeLabels = map[Mode]string{
	ModeLibrary:   "Library",
	ModeProject:   "Project",
	ModeOperation: "Operation",
	ModeHistory:   "History",
	ModeMisc:      "Misc",
}

// modeOrder is the order the menu lists the modes in.
var modeOrder = []Mode{ModeLibrary, ModeProject, ModeOperation, ModeHistory, ModeMisc}

// projectModes are the modes that only show a Project and have no selection
// page of their own.
var projectModes = []Mode{ModeOperation, ModeHistory}

// modeLabel is the name of mode, "" for a mode the menu has no row for.
func modeLabel(mode Mode) string { return modeLabels[mode] }

// hasPage reports whether mode has a selection page: the menu offers the way to
// it beside the entities of the Library and the Project. Operation and History
// have none — they only show a Project (task 73, task 89).
func hasPage(mode Mode) bool { return mode == ModeLibrary || mode == ModeProject }

// projectKey is the LocalStorage key of the Project a mode shows. The Project
// mode keeps it with its selection page (picker.ProjectKey); Operation and
// History keep their own, as their switcher did before the menu replaced it
// (task 89).
func projectKey(mode Mode) string { return string(mode) + ":projectID" }

// boolText is an attribute value for a boolean state: go-app drops an empty
// one, which would read as neither true nor false.
func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ── Opening and closing ──────────────────────────────────────────────────────

// toggleMenu opens the main menu on the mode shown, or closes it: the burger
// and the navigation line both do this.
func (r *Root) toggleMenu(ctx app.Context) {
	if r.menuOpen {
		r.closeMenu()
		return
	}
	r.openMenu(ctx)
}

// openMenu shows the menu with the mode shown expanded, and reads the entities
// afresh: what the menu lists is what the server has now (task 89).
func (r *Root) openMenu(ctx app.Context) {
	r.menuOpen = true
	r.menuExpanded = r.currentMode
	r.loadMenuEntities(ctx)
	ctx.Update()
	ctx.After(menuScrollDelay, func(app.Context) { r.scrollToShown() })
}

// scrollToShown scrolls the panel to the row of what the mode shows — the
// entity, or the way to the page while the page is shown — so a long list does
// not hide it (task 89). The Misc section scrolls to the page it shows instead
// of an entity (task 90).
func (r *Root) scrollToShown() {
	target := ""
	switch {
	case r.currentMode == ModeMisc:
		if r.miscPage != misc.PageContents {
			target = "gs-menu-misc-" + string(r.miscPage)
		}
	case hasPage(r.currentMode) && r.showList[r.currentMode]:
		target = "gs-menu-choose-" + string(r.currentMode)
	default:
		if id := r.selected[r.currentMode]; id != uuid.Nil {
			target = "gs-menu-entity-" + id.String()
		}
	}
	if target == "" {
		return
	}
	row := app.Window().Get("document").Call("getElementById", target)
	if !row.Truthy() {
		return
	}
	row.Call("scrollIntoView", map[string]any{"block": "nearest"})
}

// closeMenu hides the menu; nothing about the mode shown changes.
func (r *Root) closeMenu() {
	if !r.menuOpen {
		return
	}
	r.menuOpen = false
	if r.mounted {
		r.compoCtx.Update()
	}
}

// expandMode expands the entities of mode in the menu, or folds them. The mode
// shown is untouched until an entity or a page is chosen (task 89).
func (r *Root) expandMode(ctx app.Context, mode Mode) {
	if r.menuExpanded == mode {
		r.menuExpanded = ModeUnknown
	} else {
		r.menuExpanded = mode
	}
	ctx.Update()
}

// focusMenuButton gives the focus back to the burger, where the keyboard was
// when Esc closed the menu (task 89).
func (r *Root) focusMenuButton() {
	doc := app.Window().Get("document")
	button := doc.Call("getElementById", menuButtonID)
	if !button.Truthy() {
		return
	}
	button.Call("focus")
}

// ── The entities of the menu ─────────────────────────────────────────────────

// loadMenuEntities reads the libraries and the projects the menu lists, with
// the badges of their state (task 89).
func (r *Root) loadMenuEntities(ctx app.Context) {
	r.loadMenuLibraries(ctx)
	r.loadMenuProjects(ctx)
}

// loadMenuLibraries reads the libraries of the menu.
func (r *Root) loadMenuLibraries(ctx app.Context) { r.menuLibraries.load(ctx, r.loadMenuLibraries) }

// loadMenuProjects reads the projects of the menu: those of the Project mode
// and, with them, those Operation and History show (task 89).
func (r *Root) loadMenuProjects(ctx app.Context) { r.menuProjects.load(ctx, r.loadMenuProjects) }

// menuListOf returns the list a mode's entities live in: the libraries, or the
// projects of the three modes that show one.
func (r *Root) menuListOf(mode Mode) *menuList {
	if mode == ModeLibrary {
		return &r.menuLibraries
	}
	return &r.menuProjects
}

// menuList is one list the menu shows: the entities of a mode with the badges
// of their state, the order the person placed them in (ADR 0024), and the
// loader that keeps them fresh while the panel is open.
type menuList struct {
	source   picker.Source
	items    []picker.Item
	order    picker.Order
	loaded   bool
	reloader uiutil.Reloader
}

// load (re)reads the list and, with it, the order it is shown in.
func (l *menuList) load(ctx app.Context, reload func(app.Context)) {
	uiutil.Load(ctx, &l.reloader, l.source.List, func(ctx app.Context, items []picker.Item) {
		l.items, l.loaded = items, true
		l.loadOrder(ctx)
		ctx.Update()
	}, reload)
}

// loadOrder reads the order the person placed the entities in, so the menu
// lists them as the selection page does (task 81, ADR 0024). A source without
// one leaves the list by name; a server without the setting is not an error.
func (l *menuList) loadOrder(ctx app.Context) {
	orderSource, ok := l.source.(picker.OrderSource)
	if !ok {
		l.order = picker.Order{}
		return
	}
	uiutil.Call(ctx, orderSource.Order, func(ctx app.Context, order picker.Order) {
		l.order = order
		ctx.Update()
	}, func(ctx app.Context, err error) {
		if errors.Is(err, apiclient.ErrNotFound) {
			l.order = picker.Order{}
			ctx.Update()
			return
		}
		uiutil.ShowError(ctx, err)
	})
}

// shown returns the entities as the menu lists them.
func (l *menuList) shown() []picker.Item { return picker.Ordered(l.items, l.order) }

// empty reports whether the list has no entity to offer: either nothing was
// read yet — the panel says it is loading — or the server has none.
func (l *menuList) empty() (empty, loaded bool) { return len(l.items) == 0, l.loaded }

// ── Choosing ─────────────────────────────────────────────────────────────────

// chooseEntity opens item in mode: the editor of a Library or a Project, with
// the selection page following it; the Project of Operation or History (they
// have no page). Either switches the mode and closes the menu (task 89).
func (r *Root) chooseEntity(ctx app.Context, mode Mode, item picker.Item) {
	if !hasPage(mode) {
		r.chooseProject(ctx, mode, item)
		return
	}
	if r.selected[mode] == item.ID && !r.showList[mode] {
		// The entity the mode already shows is chosen again: switching to it
		// reloads nothing (task 89).
		r.closeMenu()
		r.SetMode(mode)
		r.storeMode()
		return
	}
	r.closeMenu()
	r.entityName[mode] = item.Name
	r.openEntity(mode, item.ID)
	r.SetMode(mode)
	r.storeMode()
	if m := r.modeView(mode); m != nil && m.picker != nil {
		m.picker.Select(ctx, item.ID)
	}
}

// choosePage switches to mode and shows its selection page, where the entities
// are created and arranged (task 73, task 89).
func (r *Root) choosePage(ctx app.Context, mode Mode) {
	r.closeMenu()
	r.SetMode(mode)
	r.showListPage(mode)
	r.storeMode()
}

// chooseProject shows item in mode, one of those without a selection page
// (Operation, History): the mode keeps it and the next reload restores it
// (task 89).
func (r *Root) chooseProject(ctx app.Context, mode Mode, item picker.Item) {
	r.closeMenu()
	r.entityName[mode] = item.Name
	r.selectProject(mode, item.ID)
	r.SetMode(mode)
	r.storeMode()
	uiutil.StoreID(r.compoCtx, projectKey(mode), item.ID)
}

// storeMode keeps the mode shown for the next reload of the page. The writes
// of the menu go through the component's own context: the element that fired
// the click is gone with the menu, and go-app drops what such a context does
// (see the note on compoCtx).
func (r *Root) storeMode() {
	r.compoCtx.LocalStorage().Set("root:mode", string(r.currentMode))
}

// ── The projects of Operation and History ────────────────────────────────────

// restoreProjects reads the names of the projects Operation and History kept
// from the last session, and drops one the server no longer has: the mode then
// shows no project instead of an empty screen named after a deleted one (task
// 89). A failed read says nothing — the modes' own reads report the server.
func (r *Root) restoreProjects(ctx app.Context) {
	kept := false
	for _, mode := range projectModes {
		if r.selected[mode] != uuid.Nil {
			kept = true
		}
	}
	if !kept {
		return
	}
	uiutil.Call(r.compoCtx, r.menuProjects.source.List, func(ctx app.Context, items []picker.Item) {
		byID := make(map[uuid.UUID]picker.Item, len(items))
		for _, item := range items {
			byID[item.ID] = item
		}
		for _, mode := range projectModes {
			id := r.selected[mode]
			if id == uuid.Nil {
				continue
			}
			if item, ok := byID[id]; ok {
				r.entityName[mode] = item.Name
				continue
			}
			r.selectProject(mode, uuid.Nil)
			uiutil.StoreID(ctx, projectKey(mode), uuid.Nil)
		}
		ctx.Update()
	}, func(app.Context, error) {})
}

// projectGoneOperation drops the project a read found gone from Operation and
// History: the project modes have no page to fall back to, so they keep none —
// the navigation line names the mode alone until another project is chosen
// (task 89).
func (r *Root) projectGoneOperation(id uuid.UUID) {
	for _, mode := range projectModes {
		if r.selected[mode] != id {
			continue
		}
		r.selectProject(mode, uuid.Nil)
		uiutil.StoreID(r.compoCtx, projectKey(mode), uuid.Nil)
	}
	if r.mounted {
		r.compoCtx.Update()
	}
}

// handleProjectEvents follows the project Operation or History shows: renamed
// elsewhere, it is renamed in the navigation line; deleted, the mode keeps no
// project at all (task 89).
func (r *Root) handleProjectEvents(ctx app.Context, ev apiv0.Event) {
	if ev.Type != apiv0.EventProjectUpdated && ev.Type != apiv0.EventProjectDeleted {
		return
	}
	id := uiutil.ParseID(ev.ID)
	for _, mode := range projectModes {
		if id == uuid.Nil || r.selected[mode] != id {
			continue
		}
		if ev.Type == apiv0.EventProjectDeleted {
			r.selectProject(mode, uuid.Nil)
			uiutil.StoreID(ctx, projectKey(mode), uuid.Nil)
			continue
		}
		r.loadProjectName(mode)
	}
}

// loadProjectName rereads the name of the project mode shows, as a rename
// elsewhere announces it. A failed read leaves the name as it was.
func (r *Root) loadProjectName(mode Mode) {
	id := r.selected[mode]
	if id == uuid.Nil {
		return
	}
	uiutil.Call(r.compoCtx, func(c context.Context) (apiv0.ProjectResponse, error) {
		return r.client.GetProject(c, id)
	}, func(ctx app.Context, project apiv0.ProjectResponse) {
		if r.selected[mode] != id {
			return
		}
		r.entityName[mode] = project.Name
		ctx.Update()
	}, func(app.Context, error) {})
}

// ── Render ───────────────────────────────────────────────────────────────────

// renderMenuBackdrop is the click outside the menu that closes it: a
// transparent sheet over everything but the header (task 89).
func (r *Root) renderMenuBackdrop() app.UI {
	if !r.menuOpen {
		return app.Div().Style("display", "none")
	}
	return app.Div().
		Attr("data-main-menu-backdrop", "true").
		Style("position", "fixed").
		Style("inset", "0").
		Style("z-index", "25").
		OnClick(func(ctx app.Context, e app.Event) {
			r.closeMenu()
			r.focusMenuButton()
		})
}

// navStep is one button of the navigation line: what it says and where it goes
// (task 91). A nil onClick is the step shown: it names the place but goes
// nowhere above itself.
type navStep struct {
	text    string
	title   string
	onClick func(ctx app.Context, e app.Event)
}

// historyHost is the mode shown while it has a saved-states screen, nil for a
// mode without one: both commands over that screen read the same mode (task 91).
type historyHost interface {
	HistoryShown() bool
	CloseHistory()
}

// historyHostOf is the mode shown, or nil when it has no saved-states screen.
func (r *Root) historyHostOf() historyHost {
	switch r.currentMode {
	case ModeLibrary:
		if r.library != nil {
			return r.library
		}
	case ModeProject:
		if r.project != nil {
			return r.project
		}
	}
	return nil
}

// historyShown reports whether the mode shown is on its saved states screen.
func (r *Root) historyShown() bool {
	host := r.historyHostOf()
	return host != nil && host.HistoryShown()
}

// closeHistory returns the mode shown from its saved states to the editor, as
// the navigation step above them does (task 91).
func (r *Root) closeHistory() {
	if host := r.historyHostOf(); host != nil {
		host.CloseHistory()
	}
	if r.mounted {
		r.compoCtx.Update()
	}
}

// navigationSteps is the chain the line beside the burger draws: the mode, the
// entity it shows and the saved states when they are open (task 91). Every
// step above the last goes one level up: the entity closes the saved states,
// the mode shows the selection page. historyShown says whether the mode shown
// is on its saved states screen, which only that mode knows.
func (r *Root) navigationSteps(historyShown bool) []navStep {
	label := modeLabel(r.currentMode)
	if label == "" {
		return nil
	}
	if r.currentMode == ModeMisc {
		page := ""
		switch r.miscPage {
		case misc.PageLabels:
			page = "Labels"
		case misc.PageUserName:
			page = "User name"
		}
		if page == "" {
			return []navStep{{text: label}}
		}
		return []navStep{
			{text: label, title: "Back to the Misc section", onClick: func(ctx app.Context, e app.Event) {
				r.misc.Show(misc.PageContents)
			}},
			{text: page},
		}
	}
	// The title says where the step goes, so only a step that goes somewhere
	// states one: Operation and History have no selection page to show.
	modeStep := navStep{text: label}
	if hasPage(r.currentMode) {
		if r.showList[r.currentMode] {
			// The selection page names no entity: the mode is what is shown.
			return []navStep{{text: label}}
		}
		modeStep.title = "Back to the selection page"
		modeStep.onClick = func(ctx app.Context, e app.Event) { r.showListPage(r.currentMode) }
	}
	steps := []navStep{modeStep}
	name := r.entityName[r.currentMode]
	switch {
	case historyShown:
		if name != "" {
			steps = append(steps, navStep{text: name, title: "Back to the Draft", onClick: func(ctx app.Context, e app.Event) {
				r.closeHistory()
			}})
		}
		steps = append(steps, navStep{text: "Saved states"})
	case name != "":
		steps = append(steps, navStep{text: name})
	}
	return steps
}

// navigationText is what the line beside the burger says: the steps of the
// chain, separated by the middle dot (task 89, task 91).
func (r *Root) navigationText() string {
	steps := r.navigationSteps(r.historyShown())
	parts := make([]string, len(steps))
	for i, step := range steps {
		parts[i] = step.text
	}
	return strings.Join(parts, middleDot)
}

// renderNavigation is the line beside the burger: one button per step of the
// chain, each going one level up (task 91).
func (r *Root) renderNavigation() app.UI {
	steps := r.navigationSteps(r.historyShown())
	text := r.navigationText()
	body := make([]app.UI, 0, len(steps)*2)
	for i, step := range steps {
		if i > 0 {
			body = append(body, app.Span().Style("color", "var(--text-muted)").Text(middleDot))
		}
		body = append(body, r.renderNavigationStep(step))
	}
	return app.Div().
		Attr("id", navigationID).
		Attr("data-navigation", "true").
		Attr("role", "navigation").
		Attr("aria-label", "Navigation: "+text).
		Title(text).
		Style("display", "flex").
		Style("align-items", "center").
		Style("min-width", "0").
		Style("padding", "4px 4px").
		Body(body...)
}

// renderNavigationStep draws one step: a button going one level up, or the
// place shown, which has nothing above it (task 91).
func (r *Root) renderNavigationStep(step navStep) app.UI {
	b := app.Button().
		Attr("data-nav-step", step.text).
		Title(step.title).
		Style("padding", "4px 6px").
		Style("border", "none").
		Style("background", "transparent").
		Style("color", "var(--text)").
		Style("font-size", "13px").
		Style("font-weight", "600").
		Style("user-select", "none")
	if step.onClick == nil {
		b = b.Disabled(true).Attr("aria-current", "page").
			Style("cursor", "default").Style("opacity", "1")
	} else {
		b = b.Style("cursor", "pointer").OnClick(step.onClick)
	}
	return b.Body(app.Span().
		Style("overflow", "hidden").
		Style("text-overflow", "ellipsis").
		Style("white-space", "nowrap").
		Text(step.text))
}

// renderMenuButton is the burger of the header (task 89).
func (r *Root) renderMenuButton() app.UI {
	return uiutil.IconButton(uiutil.IconMenu(), "", true, "Main menu", uiutil.ButtonBar,
		func(ctx app.Context, e app.Event) { r.toggleMenu(ctx) }).
		Attr("id", menuButtonID).
		Attr("aria-haspopup", "true").
		Attr("aria-expanded", boolText(r.menuOpen)).
		Attr("aria-controls", menuPanelID).
		Style("width", "30px").
		Style("height", "30px").
		Style("flex-shrink", "0")
}

// renderMenuPanel is the open menu: the modes, the current one expanded, and
// the entities of the expanded one (task 89).
func (r *Root) renderMenuPanel() app.UI {
	if !r.menuOpen {
		return app.Div().Style("display", "none")
	}
	rows := make([]app.UI, 0, len(modeOrder)*2)
	for _, mode := range modeOrder {
		rows = append(rows, r.renderModeRow(mode))
		if r.menuExpanded == mode {
			rows = append(rows, r.renderMenuEntities(mode))
		}
	}
	return app.Div().
		Attr("id", menuPanelID).
		Attr("data-main-menu", "true").
		Attr("aria-label", "Main menu").
		Style("position", "absolute").
		Style("top", "100%").
		Style("left", "0").
		Style("z-index", "1").
		Style("width", "min("+menuPanelWidth+", 100vw)").
		Style("max-height", "70vh").
		Style("overflow-y", "auto").
		Style("background", "var(--bg-elevated)").
		Style("border", "1px solid var(--border)").
		Style("border-top", "none").
		Style("box-shadow", "0 6px 18px rgba(0,0,0,0.25)").
		Style("padding", "4px 0").
		Body(rows...)
}

// renderModeRow is one row of the mode accordion: a click expands or folds the
// entities of the mode, never switches it (task 89).
func (r *Root) renderModeRow(mode Mode) app.UI {
	expanded := r.menuExpanded == mode
	color, weight := "var(--text)", "600"
	if r.currentMode == mode {
		color = "var(--accent)"
	}
	chevron := "▸"
	if expanded {
		chevron = "▾"
	}
	row := app.Button().
		Attr("data-menu-mode", string(mode)).
		Attr("aria-expanded", boolText(expanded)).
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "8px").
		Style("width", "100%").
		Style("padding", "7px 14px").
		Style("border", "none").
		Style("background", "transparent").
		Style("color", color).
		Style("font-size", "13px").
		Style("font-weight", weight).
		Style("text-align", "left").
		Style("cursor", "pointer").
		Style("user-select", "none").
		Body(
			app.Span().Text(modeLabel(mode)),
			app.Span().Style("margin-left", "auto").Style("color", "var(--text-3)").Text(chevron),
		).
		OnClick(func(ctx app.Context, e app.Event) { r.expandMode(ctx, mode) })
	if r.modeEnterable(mode) {
		return row
	}
	// Operation and History show a Project and nothing without one: with none
	// on the server there is nothing to enter, and the row says why (task 89).
	return row.
		Title("No projects yet").
		Style("opacity", "0.5").
		Style("cursor", "default").
		Disabled(true)
}

// modeEnterable reports whether the menu offers mode at all: the Library, the
// Project and Misc always (their pages create entities or belong to no
// server's list), Operation and History while the server has a project to show
// — the row waits while the list is still being read, and is off once the read
// finds none (task 89, task 90).
func (r *Root) modeEnterable(mode Mode) bool {
	if hasPage(mode) || mode == ModeMisc {
		return true
	}
	empty, loaded := r.menuListOf(mode).empty()
	return !loaded || !empty
}

// renderMenuEntities is the block under an expanded mode: the way to its
// selection page for the Library and the Project, then the entities it has
// (task 89). Misc lists its two pages instead (task 90).
func (r *Root) renderMenuEntities(mode Mode) app.UI {
	if mode == ModeMisc {
		return r.renderMiscEntities()
	}
	rows := make([]app.UI, 0, 8)
	if hasPage(mode) {
		rows = append(rows, r.renderChooseRow(mode))
	}
	list := r.menuListOf(mode)
	for _, item := range list.shown() {
		rows = append(rows, r.renderEntityRow(mode, item))
	}
	if empty, loaded := list.empty(); empty {
		rows = append(rows, renderMenuHint(mode, loaded))
	}
	return app.Div().
		Attr("data-menu-entities", string(mode)).
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("padding-bottom", "4px").
		Body(rows...)
}

// renderMiscEntities is the block under Misc: its two pages, the page shown
// marked and checked (task 90).
func (r *Root) renderMiscEntities() app.UI {
	return app.Div().
		Attr("data-menu-entities", string(ModeMisc)).
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("padding-bottom", "4px").
		Body(
			r.renderMiscRow(misc.PageLabels, "Labels", "Manage the Organization's labels"),
			r.renderMiscRow(misc.PageUserName, "User name", "The name a Save signs a saved state with"),
		)
}

// renderMiscRow is one page of the Misc section in the menu.
func (r *Root) renderMiscRow(page misc.Page, label, title string) app.UI {
	chosen := r.currentMode == ModeMisc && r.miscPage == page
	background, color := "transparent", "var(--text)"
	if chosen {
		background, color = "var(--accent-bg)", "var(--accent)"
	}
	body := []app.UI{app.Span().Text(label)}
	if chosen {
		body = append(body, app.Span().Style("margin-left", "auto").Body(uiutil.IconCheck()))
	}
	return app.Button().
		Attr("id", "gs-menu-misc-"+string(page)).
		Attr("data-menu-misc", string(page)).
		Attr("role", "menuitemradio").
		Attr("aria-checked", boolText(chosen)).
		Title(title).
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "8px").
		Style("width", "100%").
		Style("padding", "6px 14px 6px 26px").
		Style("border", "none").
		Style("background", background).
		Style("color", color).
		Style("font-size", "13px").
		Style("text-align", "left").
		Style("cursor", "pointer").
		Body(body...).
		OnClick(func(ctx app.Context, e app.Event) { r.openMiscPage(page) })
}

// renderChooseRow is the way to the selection page of a mode, where the
// entities are created, renamed and deleted (task 73, task 89).
func (r *Root) renderChooseRow(mode Mode) app.UI {
	label, title := "Choose a library…", "Show the page that lists the libraries"
	if mode == ModeProject {
		label, title = "Choose a project…", "Show the page that lists the projects"
	}
	shown := r.currentMode == mode && r.showList[mode]
	body := []app.UI{app.Span().Text(label)}
	if shown {
		body = append(body, app.Span().Style("margin-left", "auto").Body(uiutil.IconCheck()))
	}
	return app.Button().
		Attr("id", "gs-menu-choose-"+string(mode)).
		Attr("data-menu-choose", string(mode)).
		Attr("role", "menuitemradio").
		Attr("aria-checked", boolText(shown)).
		Title(title).
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "8px").
		Style("width", "100%").
		Style("padding", "6px 14px 6px 26px").
		Style("border", "none").
		Style("background", "transparent").
		Style("color", "var(--accent)").
		Style("font-size", "12px").
		Style("font-weight", "600").
		Style("text-align", "left").
		Style("cursor", "pointer").
		Body(body...).
		OnClick(func(ctx app.Context, e app.Event) { r.choosePage(ctx, mode) })
}

// renderEntityRow is one entity of the menu, with the badge of its state: the
// entity the mode shows is marked and checked (task 89).
func (r *Root) renderEntityRow(mode Mode, item picker.Item) app.UI {
	chosen := r.currentMode == mode && r.selected[mode] == item.ID && !(hasPage(mode) && r.showList[mode])
	background, color := "transparent", "var(--text)"
	if chosen {
		background, color = "var(--accent-bg)", "var(--accent)"
	}
	body := []app.UI{app.Span().
		Style("overflow", "hidden").
		Style("text-overflow", "ellipsis").
		Style("white-space", "nowrap").
		Text(item.Name)}
	if item.State != "" {
		body = append(body, picker.StateChip(item))
	}
	if chosen {
		body = append(body, app.Span().Style("flex-shrink", "0").Body(uiutil.IconCheck()))
	}
	return app.Button().
		Attr("id", "gs-menu-entity-"+item.ID.String()).
		Attr("data-menu-entity", item.ID.String()).
		Attr("role", "menuitemradio").
		Attr("aria-checked", boolText(chosen)).
		Title(item.Name).
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "8px").
		Style("width", "100%").
		Style("padding", "6px 14px 6px 26px").
		Style("border", "none").
		Style("background", background).
		Style("color", color).
		Style("font-size", "13px").
		Style("text-align", "left").
		Style("cursor", "pointer").
		Body(body...).
		OnClick(func(ctx app.Context, e app.Event) { r.chooseEntity(ctx, mode, item) })
}

// renderMenuHint is what an expanded mode without entities says: that the
// server has none, or that the menu is still reading them (task 89).
func renderMenuHint(mode Mode, loaded bool) app.UI {
	plural := "libraries"
	if mode != ModeLibrary {
		plural = "projects"
	}
	text := "No " + plural + " yet"
	if !loaded {
		text = "Loading " + plural + "…"
	}
	return app.Div().
		Attr("data-menu-hint", string(mode)).
		Style("padding", "4px 14px 6px 26px").
		Style("color", "var(--text-3)").
		Style("font-size", "12px").
		Text(text)
}
