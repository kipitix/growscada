package root

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/picker"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The main menu (task 89): the mode shown and the entity it shows are chosen
// in the menu behind the burger instead of the tabs the header used to carry.
// A row of a mode expands the entities of that mode; choosing one switches the
// mode and closes the menu. The navigation line beside the burger says what is
// shown — the mode and the entity, separated by a middle dot.
const (
	// menuButtonID and menuPanelID tie the burger to the panel it shows, for
	// aria-controls and for the focus the menu gives back.
	menuButtonID = "gs-main-menu-button"
	menuPanelID  = "gs-main-menu"
	// navigationID is the navigation line beside the burger.
	navigationID = "gs-main-navigation"
	// middleDot separates the mode from the entity named beside it: the dot
	// sits at the middle of the line's height.
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
}

// modeOrder is the order the menu lists the modes in.
var modeOrder = []Mode{ModeLibrary, ModeProject, ModeOperation, ModeHistory}

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
// not hide it (task 89).
func (r *Root) scrollToShown() {
	target := ""
	switch {
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
func (r *Root) loadMenuLibraries(ctx app.Context) {
	uiutil.Load(r.compoCtx, &r.menuLibraryReloader, r.menuLibrarySource.List,
		func(ctx app.Context, items []picker.Item) {
			r.menuLibraries, r.menuLibrariesLoaded = items, true
			r.loadMenuOrder(ctx, r.menuLibrarySource, func(order picker.Order) {
				r.menuLibraryOrder = order
			})
			ctx.Update()
		}, r.loadMenuLibraries)
}

// loadMenuProjects reads the projects of the menu: those of the Project mode
// and, with them, those Operation and History show (task 89).
func (r *Root) loadMenuProjects(ctx app.Context) {
	uiutil.Load(r.compoCtx, &r.menuProjectReloader, r.menuProjectSource.List,
		func(ctx app.Context, items []picker.Item) {
			r.menuProjects, r.menuProjectsLoaded = items, true
			r.loadMenuOrder(ctx, r.menuProjectSource, func(order picker.Order) {
				r.menuProjectOrder = order
			})
			ctx.Update()
		}, r.loadMenuProjects)
}

// loadMenuOrder reads the order the person placed the entities in, so the menu
// lists them as the selection page does (task 81, ADR 0024). A source without
// one leaves the list by name; a server without the setting is not an error.
func (r *Root) loadMenuOrder(ctx app.Context, source picker.Source, keep func(picker.Order)) {
	orderSource, ok := source.(picker.OrderSource)
	if !ok {
		keep(picker.Order{})
		return
	}
	uiutil.Call(r.compoCtx, orderSource.Order, func(ctx app.Context, order picker.Order) {
		keep(order)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		if errors.Is(err, apiclient.ErrNotFound) {
			keep(picker.Order{})
			ctx.Update()
			return
		}
		uiutil.ShowError(ctx, err)
	})
}

// menuItems returns the entities of mode as the menu lists them.
func (r *Root) menuItems(mode Mode) []picker.Item {
	if mode == ModeLibrary {
		return picker.Ordered(r.menuLibraries, r.menuLibraryOrder)
	}
	return picker.Ordered(r.menuProjects, r.menuProjectOrder)
}

// menuEmpty reports whether a mode of the menu has no entity to offer: either
// nothing was read yet — the panel says it is loading — or the server has none.
func (r *Root) menuEmpty(mode Mode) (empty, loaded bool) {
	if mode == ModeLibrary {
		return len(r.menuLibraries) == 0, r.menuLibrariesLoaded
	}
	return len(r.menuProjects) == 0, r.menuProjectsLoaded
}

// ── Choosing ─────────────────────────────────────────────────────────────────

// chooseEntity opens item in mode: the editor of a Library or a Project, with
// the selection page following it; the Project of Operation or History (they
// have no page). Either switches the mode and closes the menu (task 89).
func (r *Root) chooseEntity(ctx app.Context, mode Mode, item picker.Item) {
	if !hasPage(mode) {
		r.chooseProject(ctx, mode, item)
		return
	}
	r.closeMenu()
	r.openEntityAs(mode, item.ID, item.Name)
	r.SetMode(mode)
	r.storeMode()
	if m := r.modeView(mode); m != nil && m.picker != nil {
		m.picker.Show(ctx, item.ID)
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
	uiutil.Call(r.compoCtx, r.client.ListProjects, func(ctx app.Context, list []apiv0.ProjectResponse) {
		byID := make(map[uuid.UUID]apiv0.ProjectResponse, len(list))
		for _, project := range list {
			byID[project.ID] = project
		}
		for _, mode := range projectModes {
			id := r.selected[mode]
			if id == uuid.Nil {
				continue
			}
			if project, ok := byID[id]; ok {
				r.entityName[mode] = project.Name
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
		OnClick(func(ctx app.Context, e app.Event) { r.closeMenu() })
}

// navigationText is what the line beside the burger says: the mode and the
// entity it shows, separated by the middle dot, the mode alone when no entity
// is known yet or the mode shows its selection page (task 89).
func (r *Root) navigationText() string {
	label := modeLabel(r.currentMode)
	if label == "" {
		return ""
	}
	if hasPage(r.currentMode) && r.showList[r.currentMode] {
		// The selection page names no entity: the mode is what is shown.
		return label
	}
	if name := r.entityName[r.currentMode]; name != "" {
		return label + middleDot + name
	}
	return label
}

// renderNavigation is the line beside the burger; it opens the menu as the
// burger does (task 89).
func (r *Root) renderNavigation() app.UI {
	text := r.navigationText()
	return app.Button().
		Attr("id", navigationID).
		Attr("data-navigation", "true").
		Attr("aria-label", "Main menu: "+text).
		Attr("aria-haspopup", "true").
		Attr("aria-expanded", boolText(r.menuOpen)).
		Attr("aria-controls", menuPanelID).
		Title(text).
		Style("display", "flex").
		Style("align-items", "center").
		Style("min-width", "0").
		Style("padding", "4px 8px").
		Style("border", "none").
		Style("background", "transparent").
		Style("color", "var(--text)").
		Style("font-size", "13px").
		Style("font-weight", "600").
		Style("cursor", "pointer").
		Style("user-select", "none").
		Body(app.Span().
			Style("overflow", "hidden").
			Style("text-overflow", "ellipsis").
			Style("white-space", "nowrap").
			Text(text)).
		OnClick(func(ctx app.Context, e app.Event) { r.toggleMenu(ctx) })
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

// modeEnterable reports whether the menu offers mode at all: the Library and
// the Project always (their page creates entities), Operation and History only
// while the server has a project to show — nothing is offered while the list is
// still being read (task 89).
func (r *Root) modeEnterable(mode Mode) bool {
	if hasPage(mode) {
		return true
	}
	empty, loaded := r.menuEmpty(mode)
	return !loaded || !empty
}

// renderMenuEntities is the block under an expanded mode: the way to its
// selection page for the Library and the Project, then the entities it has
// (task 89).
func (r *Root) renderMenuEntities(mode Mode) app.UI {
	rows := make([]app.UI, 0, 8)
	if hasPage(mode) {
		rows = append(rows, r.renderChooseRow(mode))
	}
	items := r.menuItems(mode)
	for _, item := range items {
		rows = append(rows, r.renderEntityRow(mode, item))
	}
	if empty, loaded := r.menuEmpty(mode); empty {
		rows = append(rows, renderMenuHint(mode, loaded))
	}
	return app.Div().
		Attr("data-menu-entities", string(mode)).
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("padding-bottom", "4px").
		Body(rows...)
}

// renderChooseRow is the way to the selection page of a mode, where the
// entities are created, renamed and deleted (task 73, task 89).
func (r *Root) renderChooseRow(mode Mode) app.UI {
	label, title := "Choose a library…", "Show the page that lists the libraries"
	if mode == ModeProject {
		label, title = "Choose a project…", "Show the page that lists the projects"
	}
	shown := r.showList[mode]
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
	chosen := r.selected[mode] == item.ID && !(hasPage(mode) && r.showList[mode])
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
		badgeColor, border := picker.StateBadge(item.StateKind)
		body = append(body, app.Span().
			Attr("data-menu-state", item.StateKind).
			Title(item.State).
			Style("flex-shrink", "0").
			Style("padding", "0 6px").
			Style("border", "1px solid "+border).
			Style("border-radius", "8px").
			Style("color", badgeColor).
			Style("font-size", "10px").
			Text(item.State))
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
