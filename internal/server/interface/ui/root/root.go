package root

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/history"
	"github.com/kipitix/growscada/internal/server/interface/ui/labels"
	"github.com/kipitix/growscada/internal/server/interface/ui/library"
	"github.com/kipitix/growscada/internal/server/interface/ui/misc"
	"github.com/kipitix/growscada/internal/server/interface/ui/operation"
	"github.com/kipitix/growscada/internal/server/interface/ui/picker"
	"github.com/kipitix/growscada/internal/server/interface/ui/project"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

type Mode string

const (
	ModeUnknown   Mode = ""
	ModeLibrary   Mode = "library"
	ModeProject   Mode = "project"
	ModeOperation Mode = "operation"
	ModeHistory   Mode = "history"
	ModeMisc      Mode = "misc"
)

// The screens of a mode with a selection page, as LocalStorage keeps the one
// the person left (task 85_1): the page of entities or the editor of one.
const (
	screenList   = "list"
	screenEditor = "editor"
)

// screenKey is the LocalStorage key of the screen of mode ("library:screen",
// "project:screen"); absent, the screen follows the entity selected, as task 73
// had it.
func screenKey(mode Mode) string { return string(mode) + ":screen" }

// miscPageKey is the LocalStorage key of the page of the Misc section shown
// (task 90).
const miscPageKey = "misc:page"

type Root struct {
	app.Compo
	// compoCtx is the component's own context, captured in OnMount; the server
	// version seen by the client is dispatched through it.
	compoCtx    app.Context
	mounted     bool
	currentMode Mode
	themeMode   string // "auto" | "light" | "dark"
	// client is the one client of the REST API, given to every mode.
	client    *apiclient.Client
	project   *project.Project
	library   *library.Library
	operation *operation.Operation
	// bar follows the event stream of the project of the mode shown (the
	// last such mode while Library, which does not depend on one, is shown).
	bar *eventlog.Bar
	// misc is the Misc mode (task 90): the Labels page and the User name page,
	// one of which miscPage names.
	misc     *misc.Section
	miscPage misc.Page
	// selected holds the entity each mode shows, once the main menu or the
	// selection page announced it: the Project of Operation and History, and
	// the Project or Library of the Project and Library modes.
	selected map[Mode]uuid.UUID
	// entityName is the name of the entity selected, as last read by the
	// selection page: the slim "← List · <name>" row of the editor.
	entityName map[Mode]string
	// showList is true while a mode shows its selection page (task 73)
	// instead of the editor of the entity selected. It is decided when a mode
	// is opened the first time and then kept for the tab's lifetime, so a walk
	// from one tab to another and back returns to the same screen; LocalStorage
	// holds it too, so a reload opens the screen the person left (task 85_1).
	showList map[Mode]bool
	// The modes in the order of their slots, made once: a mode is mounted
	// when first opened and stays mounted while another is shown, so it
	// keeps what it loaded (shown even while the server is down), its
	// selection and unsaved edits, and keeps following server events.
	modes []modeView
	// opened holds the modes opened so far (SetMode).
	opened map[Mode]bool
	// serverVersion is the server API version of the last response, if
	// serverVersionKnown.
	serverVersion      contract.SchemaVersion
	serverVersionKnown bool
	// reported is the last version observeServerVersion dispatched; it is
	// read and written off the UI goroutine, hence the mutex.
	reportedMu    sync.Mutex
	reported      contract.SchemaVersion
	reportedKnown bool
	// reloadOnAppUpdate is set by the banner's Reload while the service
	// worker fetches the new app (see reloadApp).
	reloadOnAppUpdate bool
	// The main menu (task 89): menuOpen is whether it is shown, menuExpanded
	// the mode whose entities it lists; the two lists hold what it shows and
	// keep it fresh.
	menuOpen      bool
	menuExpanded  Mode
	menuLibraries menuList
	menuProjects  menuList
	// onKeyDown closes the menu on Esc, wherever the focus is (task 89).
	onKeyDown app.Func
}

func NewRoot(anAPIServerURL string) *Root {
	r := &Root{
		currentMode: ModeLibrary,
		themeMode:   "auto",
	}
	r.client = apiclient.New(anAPIServerURL, http.DefaultClient, apiclient.OnServerVersion(r.observeServerVersion))
	r.project = project.NewProject(r.client)
	r.library = library.NewLibrary(r.client)
	// The panel of each editor carries the way back to its selection page
	// (task 84): Root owns the page and the name the selection announced, so
	// the mode asks for both instead of knowing them.
	r.library.OnBack = func() { r.showListPage(ModeLibrary) }
	r.library.BackName = func() string { return r.entityName[ModeLibrary] }
	r.project.OnBack = func() { r.showListPage(ModeProject) }
	r.project.BackName = func() string { return r.entityName[ModeProject] }
	r.operation = operation.NewOperation(r.client)
	r.bar = eventlog.NewBar(anAPIServerURL)
	r.misc = misc.New(r.client)
	r.misc.OnPage = func(page misc.Page) {
		r.miscPage = page
		r.storeMiscPage(page)
		if r.mounted {
			r.compoCtx.Update()
		}
	}
	r.modes = []modeView{
		{mode: ModeLibrary, ui: r.library, picker: picker.New(
			picker.NewLibrarySource(r.client), picker.LibraryKey,
			func(id uuid.UUID, name string) { r.selectEntity(ModeLibrary, id, name) },
			func(id uuid.UUID) { r.openEntity(ModeLibrary, id) },
		)},
		{mode: ModeProject, ui: r.project, picker: picker.New(
			picker.NewProjectSource(r.client), picker.ProjectKey,
			func(id uuid.UUID, name string) { r.selectEntity(ModeProject, id, name) },
			func(id uuid.UUID) { r.openEntity(ModeProject, id) },
		)},
		{mode: ModeOperation, ui: r.operation},
		{mode: ModeHistory, ui: &history.History{}},
		{mode: ModeMisc, ui: r.misc},
	}
	// The main menu reads the entities of both selection pages itself, so it
	// does not depend on a mode having been opened (task 89).
	r.menuLibraries.source = picker.NewLibrarySource(r.client)
	r.menuProjects.source = picker.NewProjectSource(r.client)
	r.opened = map[Mode]bool{}
	r.selected = map[Mode]uuid.UUID{}
	r.entityName = map[Mode]string{}
	r.showList = map[Mode]bool{}
	return r
}

func (r *Root) OnMount(ctx app.Context) {
	r.compoCtx, r.mounted = ctx, true
	ctx.Page().SetTitle("GrowSCADA")

	// The mode shown opens the entity last shown: it is read here, before any
	// mode mounts, so the editor does not wait for its selection page (task
	// 73). Operation and History keep their Project under their own key, as
	// their switcher did until the main menu replaced it (task 89).
	r.selected[ModeProject] = uiutil.LoadID(ctx, picker.ProjectKey)
	r.selected[ModeLibrary] = uiutil.LoadID(ctx, picker.LibraryKey)
	r.selected[ModeOperation] = uiutil.LoadID(ctx, projectKey(ModeOperation))
	r.selected[ModeHistory] = uiutil.LoadID(ctx, projectKey(ModeHistory))

	// The screen of a mode with a selection page is what the person left
	// there, restored before any mode mounts: the page, or the editor of the
	// entity selected — the page when there is none, whatever was stored
	// (task 85_1).
	r.showList[ModeProject] = showsList(loadScreen(ctx, ModeProject), r.selected[ModeProject])
	r.showList[ModeLibrary] = showsList(loadScreen(ctx, ModeLibrary), r.selected[ModeLibrary])

	// The page of the Misc section too: a reload returns to Labels or User
	// name, and the section's contents when none was open (task 90).
	r.miscPage = loadMiscPage(ctx)
	r.misc.SetPage(r.miscPage)

	// A read of the entity the editor shows found it gone: the mode leaves it
	// at once, so the content the server removes after it is not loaded for
	// it, and shows its selection page with its list reread (task 73,
	// task 76). The Project says so through the content of its project, the
	// Library through a read of its Draft, its stack or its saved states.
	ctx.Handle(uiutil.ActionProjectGone, func(ctx app.Context, a app.Action) {
		if id, ok := a.Value.(uuid.UUID); ok {
			r.entityGone(ModeProject, id)
		}
	})
	ctx.Handle(uiutil.ActionLibraryGone, func(ctx app.Context, a app.Action) {
		if id, ok := a.Value.(uuid.UUID); ok {
			r.entityGone(ModeLibrary, id)
		}
	})
	// A read of the project Operation or History shows found it gone — the
	// Player's 404 of the starting state, or any read of its content: the mode
	// leaves it, so no stale name hangs in the navigation line and the next
	// reload does not look for it again (task 89).
	ctx.Handle(uiutil.ActionProjectGone, func(ctx app.Context, a app.Action) {
		if id, ok := a.Value.(uuid.UUID); ok {
			r.projectGoneOperation(id)
		}
	})
	// A mode's history screen opens the Labels page through this action: the
	// page is the Misc section's, and Root owns the page shown (task 85_3,
	// task 90). The User name page has no such action: it is reached from the
	// main menu alone, the one place the name is shown and set.
	ctx.Handle(labels.ActionOpen, func(ctx app.Context, _ app.Action) {
		r.openMiscPage(misc.PageLabels)
	})
	// The menu follows the entities it lists and the project Operation or
	// History shows: a list changes while it is open, a project is renamed or
	// deleted elsewhere (task 89).
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok {
			return
		}
		r.handleProjectEvents(ctx, ev)
		if !r.menuOpen {
			return
		}
		// A Save and a write of a Draft change the badge of a row, and the
		// event does not say which list holds it: both are reread, the
		// Reloader keeping a burst to one request each (task 89).
		targets := eventlog.ReloadTargetsFor(ev)
		if targets.Projects || targets.Libraries || targets.SavedStates || targets.DraftChanged() {
			r.loadMenuProjects(ctx)
			r.loadMenuLibraries(ctx)
		}
	})
	r.listenMenuKeys()
	r.restoreProjects(ctx)

	injectToastCSS()

	var savedMode string
	ctx.LocalStorage().Get("root:mode", &savedMode)

	var savedTheme string
	ctx.LocalStorage().Get("root:theme", &savedTheme)
	if savedTheme != "light" && savedTheme != "dark" && savedTheme != "auto" {
		savedTheme = r.themeMode // keep default "auto"
	}
	injectThemeCSS(savedTheme) // inject CSS before dispatch to minimise FOUC

	ctx.Dispatch(func(ctx app.Context) {
		r.themeMode = savedTheme
		ctx.SetState("theme", savedTheme)
		mode := r.currentMode
		if Mode(savedMode) != ModeUnknown {
			mode = Mode(savedMode)
		}
		// The Operation of the last session shows its project at once, before
		// its first mount (task 89).
		if id := r.selected[ModeOperation]; id != uuid.Nil {
			r.operation.SetProject(id)
		}
		// Only now, so a mode is not opened (and loaded) just for being
		// the default before the saved one is known.
		r.SetMode(mode)
	})
}

// OnDismount releases the key listener of the menu.
func (r *Root) OnDismount() {
	if r.onKeyDown == nil {
		return
	}
	app.Window().Call("removeEventListener", "keydown", r.onKeyDown)
	r.onKeyDown.Release()
	r.onKeyDown = nil
}

// listenMenuKeys closes the menu on Esc, from wherever the focus is, and gives
// the focus back to the burger (task 89).
func (r *Root) listenMenuKeys() {
	if r.onKeyDown != nil {
		return
	}
	r.onKeyDown = app.FuncOf(func(this app.Value, args []app.Value) any {
		if len(args) == 0 || !r.menuOpen {
			return nil
		}
		if args[0].Get("key").String() != "Escape" {
			return nil
		}
		// The menu takes the key: the mode under it keeps its selection, and
		// Esc closes the menu alone (task 89). Root listens from its own mount
		// on, before any mode's listener, so stopping here stops them too.
		args[0].Call("preventDefault")
		args[0].Call("stopImmediatePropagation")
		r.closeMenu()
		r.focusMenuButton()
		return nil
	})
	app.Window().Call("addEventListener", "keydown", r.onKeyDown)
}

// openMiscPage shows page of the Misc section, switching to it and keeping it
// for the next reload (task 90). The section reports the page through OnPage,
// which stores it and updates the navigation line; here only the mode and the
// menu follow.
func (r *Root) openMiscPage(page misc.Page) {
	r.closeMenu()
	r.misc.Show(page)
	r.SetMode(ModeMisc)
	r.storeMode()
	if r.mounted {
		r.compoCtx.Update()
	}
}

// storeMiscPage keeps the Misc page shown for the next reload: the contents
// when none is open (task 90).
func (r *Root) storeMiscPage(page misc.Page) {
	if !r.mounted {
		return
	}
	r.compoCtx.LocalStorage().Set(miscPageKey, string(page))
}

// loadMiscPage reads the Misc page a previous session left, the contents when
// none is stored or the value is unknown (task 90).
func loadMiscPage(ctx app.Context) misc.Page {
	var page string
	if err := ctx.LocalStorage().Get(miscPageKey, &page); err != nil {
		return misc.PageContents
	}
	switch misc.Page(page) {
	case misc.PageLabels, misc.PageUserName:
		return misc.Page(page)
	default:
		return misc.PageContents
	}
}

// selectProject shows the project id (uuid.Nil: none) in mode, one of those
// that only show a project (Operation, History), chosen in the main menu and
// called on the UI goroutine (task 89). Operation reads nothing of the Bar:
// its own Player follows the Journal (ADR 0020); History reads it. If the Bar
// follows the project already, it resyncs now; else, if mode is shown, the Bar
// follows it, its stream resyncing as it opens; else mode reads it once shown
// (SetMode). The Project mode has a selection page instead, which announces
// through selectEntity (task 73).
func (r *Root) selectProject(mode Mode, id uuid.UUID) {
	r.selected[mode] = id
	if id == uuid.Nil {
		r.entityName[mode] = ""
	}
	if mode == ModeOperation {
		r.operation.SetProject(id)
	}
	switch {
	case r.bar.Project() == id:
		r.bar.ResyncProject()
	case mode == r.currentMode:
		r.bar.SetProject(id)
	}
}

// selectEntity shows the entity id (uuid.Nil: none) in the Library or the
// Project mode on behalf of its selection page, which calls it on the UI
// goroutine with the name it read. A mode with nothing to show leaves the
// page open (task 73); the entity selected stays announced to the event
// stream, so the page's list and the editor follow the same one.
func (r *Root) selectEntity(mode Mode, id uuid.UUID, name string) {
	r.selected[mode] = id
	r.entityName[mode] = name
	switch mode {
	case ModeProject:
		r.project.SetProject(id)
		switch {
		case r.bar.Project() == id:
			r.bar.ResyncProject()
		case mode == r.currentMode:
			r.bar.SetProject(id)
		}
	case ModeLibrary:
		r.library.SetLibrary(id)
	}
	if id == uuid.Nil {
		r.showList[mode] = true
		r.storeScreen(mode, screenList)
	}
	if r.mounted {
		r.compoCtx.Update()
	}
}

// openEntity shows the editor of id: the selection page's Open row or a
// created entity.
func (r *Root) openEntity(mode Mode, id uuid.UUID) {
	if id == uuid.Nil {
		return
	}
	r.showList[mode] = false
	r.storeScreen(mode, screenEditor)
	r.selectEntity(mode, id, r.entityName[mode])
}

// openEntityAs is openEntity under the name the main menu read for the entity
// it opened (task 89).
func (r *Root) openEntityAs(mode Mode, id uuid.UUID, name string) {
	if id == uuid.Nil {
		return
	}
	r.entityName[mode] = name
	r.openEntity(mode, id)
}

// showListPage shows the selection page of mode, rereading its list, and
// keeps the entity selected announced to the event stream as it is.
func (r *Root) showListPage(mode Mode) {
	r.showList[mode] = true
	r.storeScreen(mode, screenList)
	if m := r.modeView(mode); m != nil && m.picker != nil && r.mounted {
		m.picker.Reload(r.compoCtx)
	}
	if r.mounted {
		r.compoCtx.Update()
	}
}

// entityGone leaves the entity id of mode, which a read found gone, and shows
// the selection page with its list reread: the editor that would be empty is
// replaced by the page, and the deleted row leaves the list instead of hanging
// there (task 76). The screen is recorded as the page, so a reload does not
// reopen the editor of an entity that is gone (task 85_1). The id must still
// be the one shown: a delayed 404 of an entity left meanwhile is not this one's.
func (r *Root) entityGone(mode Mode, id uuid.UUID) {
	if id == uuid.Nil || r.selected[mode] != id {
		return
	}
	r.selectEntity(mode, uuid.Nil, "")
	r.showListPage(mode)
}

// loadScreen reads the screen stored for mode, "" when none is: the screen
// follows the entity selected then, as task 73 had it.
func loadScreen(ctx app.Context, mode Mode) string {
	var s string
	if err := ctx.LocalStorage().Get(screenKey(mode), &s); err != nil {
		return ""
	}
	if s != screenList && s != screenEditor {
		return ""
	}
	return s
}

// storeScreen keeps the screen of mode for the next opening of its tab and the
// next reload of the page; before the mount there is no LocalStorage to write.
func (r *Root) storeScreen(mode Mode, screen string) {
	if !r.mounted {
		return
	}
	r.compoCtx.LocalStorage().Set(screenKey(mode), screen)
}

// showsList reports whether a mode with a selection page opens on the page,
// given the screen it stored and the entity selected: the stored page always,
// the stored editor only with an entity to show — an editor without one is the
// page (task 85_1) — and with nothing stored the page when no entity is
// selected and its editor when one is (task 73).
func showsList(stored string, selected uuid.UUID) bool {
	if stored == screenList {
		return true
	}
	return selected == uuid.Nil
}

// modeView returns the view of mode, nil if there is none.
func (r *Root) modeView(mode Mode) *modeView {
	for i := range r.modes {
		if r.modes[i].mode == mode {
			return &r.modes[i]
		}
	}
	return nil
}

// barProject is the project the event stream follows while mode is shown:
// none for Library, which does not depend on one, and for Misc, which leaves
// the stream of the mode before it (task 90).
func (r *Root) barProject(mode Mode) (uuid.UUID, bool) {
	if mode == ModeLibrary || mode == ModeMisc {
		return uuid.Nil, false
	}
	id, ok := r.selected[mode]
	return id, ok
}

// observeServerVersion records the server API version stated by a response.
// The client calls it off the UI goroutine, so it is dispatched; requests
// start from the modes' OnMount, which runs after Root's, so compoCtx is set.
// Only a change is dispatched: a dispatch renders Root and every mode under
// it, and every response states the version. It is dispatched under the
// lock, so the dispatches queue in the order of the changes and the last one
// leaves serverVersion at reported.
func (r *Root) observeServerVersion(v contract.SchemaVersion) {
	r.reportedMu.Lock()
	defer r.reportedMu.Unlock()
	if r.reportedKnown && r.reported == v {
		return
	}
	r.reported, r.reportedKnown = v, true
	r.compoCtx.Dispatch(func(ctx app.Context) {
		r.serverVersion, r.serverVersionKnown = v, true
	})
}

// versionMismatch reports whether the server speaks another API version than
// this UI was built for: older or newer, the UI and the server were not
// deployed together.
func (r *Root) versionMismatch() bool {
	return r.serverVersionKnown && r.serverVersion != apiv0.SchemaVersion()
}

// renderVersionBanner warns, until the versions match again, that the server
// speaks another API version than the UI: a page loaded before the server was
// updated. It cannot be dismissed but blocks nothing.
func (r *Root) renderVersionBanner() app.UI {
	if !r.versionMismatch() {
		return app.Div().Style("display", "none")
	}
	return app.Div().
		Attr("role", "alert").
		// Not "": go-app drops an attribute whose value is empty.
		Attr("data-version-banner", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "12px").
		Style("padding", "6px 12px").
		Style("background", "var(--toast-warn-border)").
		Style("color", "#fff").
		Style("font-size", "13px").
		Style("font-weight", "600").
		Body(
			app.Span().Text(versionBannerText(r.serverVersion)),
			app.Button().
				Style("padding", "2px 10px").
				Style("font-size", "12px").
				Style("cursor", "pointer").
				Text(reloadLabel(r.reloadOnAppUpdate)).
				Disabled(r.reloadOnAppUpdate).
				OnClick(func(ctx app.Context, e app.Event) {
					r.reloadApp(ctx)
				}),
		)
}

func reloadLabel(updating bool) string {
	if updating {
		return "Updating…"
	}
	return "Reload"
}

// appUpdateTimeout bounds the wait for the service worker in reloadApp.
const appUpdateTimeout = 10 * time.Second

// reloadApp reloads the page with the server's current app. The service
// worker serves the app from its cache, so a plain reload would bring back the
// same app.wasm while the new worker is not yet active: unless an update is
// already active (or no worker controls the page), the worker is asked to
// update first and the page reloads once it has (OnAppUpdate) — or after
// appUpdateTimeout, when there was nothing newer to fetch.
func (r *Root) reloadApp(ctx app.Context) {
	if ctx.AppUpdateAvailable() || !serviceWorkerControlsPage() {
		ctx.Reload()
		return
	}
	if r.reloadOnAppUpdate {
		return
	}
	r.reloadOnAppUpdate = true
	app.TryUpdate()
	ctx.After(appUpdateTimeout, func(ctx app.Context) { ctx.Reload() })
}

// OnAppUpdate is called by go-app once the service worker has activated a new
// version of the app.
func (r *Root) OnAppUpdate(ctx app.Context) {
	if r.reloadOnAppUpdate {
		ctx.Reload()
	}
}

func serviceWorkerControlsPage() bool {
	sw := app.Window().Get("navigator").Get("serviceWorker")
	return sw.Truthy() && sw.Get("controller").Truthy()
}

func versionBannerText(server contract.SchemaVersion) string {
	return fmt.Sprintf("The server speaks API %v, this interface %v. Reload the page.", server, apiv0.SchemaVersion())
}

func (r *Root) setTheme(ctx app.Context, mode string) {
	r.themeMode = mode
	ctx.LocalStorage().Set("root:theme", mode)
	injectThemeCSS(mode)
	ctx.SetState("theme", mode)
}

// ── CSS animations (toast) ────────────────────────────────────────────────────

func injectToastCSS() {
	doc := app.Window().Get("document")
	el := doc.Call("getElementById", "gs-toast-css")
	if el.Truthy() {
		return // already injected
	}
	el = doc.Call("createElement", "style")
	el.Set("id", "gs-toast-css")
	el.Set("textContent", `
@keyframes gs-toast-wrap-enter {
  from { max-height: 0; overflow: hidden; }
  to   { max-height: 800px; overflow: hidden; }
}
@keyframes gs-toast-card-enter {
  0%   { opacity: 0; }
  35%  { opacity: 0; }
  100% { opacity: 1; }
}
@keyframes gs-toast-wrap-exit {
  0%   { max-height: 800px; overflow: hidden; }
  45%  { max-height: 800px; overflow: hidden; }
  100% { max-height: 0;   overflow: hidden; }
}
@keyframes gs-toast-card-exit {
  0%   { opacity: 1; transform: translateY(0); }
  45%  { opacity: 0; transform: translateY(-22px); }
  100% { opacity: 0; transform: translateY(-22px); }
}`)
	doc.Get("head").Call("appendChild", el)
}

// ── CSS theme injection ───────────────────────────────────────────────────────

func injectThemeCSS(mode string) {
	doc := app.Window().Get("document")
	el := doc.Call("getElementById", "gs-theme-vars")
	if !el.Truthy() {
		el = doc.Call("createElement", "style")
		el.Set("id", "gs-theme-vars")
		doc.Get("head").Call("appendChild", el)
	}
	el.Set("textContent", buildThemeCSS(mode))
}

func lightVars() string {
	return `
		--bg: #ffffff;
		--bg-elevated: #f8f8f8;
		--bg-hover: #f0f0f0;
		--bg-inset: #e8e8e8;
		--surface: #fafafa;
		--border: #dddddd;
		--border-subtle: #f0f0f0;
		--border-input: #cccccc;
		--text: #222222;
		--text-2: #555555;
		--text-3: #888888;
		--text-muted: #aaaaaa;
		--accent: #0066cc;
		--accent-text: #ffffff;
		--accent-bg: #f0f4ff;
		--accent-border: #c8d8f8;
		--error: #cc0000;
		--error-bg: #fff5f5;
		--error-border: #e0b0b0;
		--input-bg: #ffffff;
		--widget-bg: rgba(240,240,240,0.85);
		--widget-sel-bg: rgba(235,245,255,0.92);
		--toast-bg: rgba(252,252,252,0.98);
		--toast-shadow: 0 4px 18px rgba(0,0,0,0.12);
		--toast-text: #222222;
		--toast-text-meta: #888888;
		--toast-text-instance: #aaaaaa;
		--toast-err-border: #cc3333;
		--toast-err-title: #cc0000;
		--toast-warn-border: #aa7700;
		--toast-warn-title: #886600;
		--toast-info-border: #2266cc;
		--toast-info-title: #0055aa;
		--toast-muted-border: #999999;
		--toast-muted-title: #555555;
		color-scheme: light;`
}

func darkVars() string {
	return `
		--bg: #1a1a1a;
		--bg-elevated: #242424;
		--bg-hover: #2e2e2e;
		--bg-inset: #111111;
		--surface: #1e1e1e;
		--border: #363636;
		--border-subtle: #2a2a2a;
		--border-input: #4a4a4a;
		--text: #e4e4e4;
		--text-2: #999999;
		--text-3: #6a6a6a;
		--text-muted: #585858;
		--accent: #4d9fff;
		--accent-text: #ffffff;
		--accent-bg: #192840;
		--accent-border: #2a4a7a;
		--error: #ff6868;
		--error-bg: #2a1818;
		--error-border: #7a3838;
		--input-bg: #262626;
		--widget-bg: rgba(36,36,36,0.92);
		--widget-sel-bg: rgba(18,38,64,0.95);
		--toast-bg: rgba(18,18,18,0.97);
		--toast-shadow: 0 4px 18px rgba(0,0,0,0.55);
		--toast-text: #cccccc;
		--toast-text-meta: #666666;
		--toast-text-instance: #666666;
		--toast-err-border: #cc3333;
		--toast-err-title: #ff6666;
		--toast-warn-border: #bb8800;
		--toast-warn-title: #ffcc33;
		--toast-info-border: #2266cc;
		--toast-info-title: #66aaff;
		--toast-muted-border: #555555;
		--toast-muted-title: #aaaaaa;
		color-scheme: dark;`
}

func buildThemeCSS(mode string) string {
	formReset := `
html, body {
	margin: 0;
	padding: 0;
	height: 100%;
	overflow: hidden;
}
input, select, textarea, button {
	background-color: var(--input-bg);
	color: var(--text);
}
button {
	background-color: var(--bg-hover);
}`

	switch mode {
	case "dark":
		return `:root {` + darkVars() + `}` + formReset
	case "light":
		return `:root {` + lightVars() + `}` + formReset
	default: // auto
		return `:root {` + lightVars() + `}
@media (prefers-color-scheme: dark) { :root {` + darkVars() + `} }` + formReset
	}
}

// ── Render ────────────────────────────────────────────────────────────────────

func (r *Root) Render() app.UI {
	r.misc.SetShown(r.currentMode == ModeMisc)
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("height", "100vh").
		Style("font-family", "sans-serif").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Body(
			&toast.Container{},
			r.renderVersionBanner(),
			r.renderMenuBackdrop(),
			r.renderHeader(),
			app.Div().
				Style("flex", "1").
				Style("min-height", "0").
				Style("position", "relative").
				Style("background", "var(--bg)").
				Body(app.Range(r.modes).Slice(func(i int) app.UI {
					return r.modeSlot(&r.modes[i])
				})),
			r.bar,
		)
}

// renderHeader is the one row above the modes: the burger of the main menu and
// the navigation line naming what is shown, then the actions every mode shares
// (task 89). It holds the open menu, so the panel hangs under it.
func (r *Root) renderHeader() app.UI {
	return app.Div().
		Attr("data-header", "true").
		Style("position", "relative").
		Style("z-index", "30").
		Style("display", "flex").
		Style("flex-direction", "row").
		Style("align-items", "center").
		Style("gap", "8px").
		Style("padding", "5px 10px").
		Style("min-height", "38px").
		Style("box-sizing", "border-box").
		Style("border-bottom", "2px solid var(--border)").
		Style("background", "var(--bg-elevated)").
		Body(
			r.renderMenuButton(),
			r.renderNavigation(),
			app.Div().Style("flex", "1"),
			r.renderThemeToggle(),
			r.renderMenuPanel(),
		)
}

// modeSlot is the fixed place of one mode: empty until the mode is first
// opened, then holding it for good. Each mode keeps its own slot, so opening
// one never remounts another. A mode not shown is hidden with visibility,
// not display: the iframes of its Widgets keep their size.
func (r *Root) modeSlot(m *modeView) app.UI {
	visibility := "hidden"
	if m.mode == r.currentMode {
		visibility = "visible"
	}
	return app.Div().
		Attr("data-mode", string(m.mode)).
		Style("position", "absolute").
		Style("inset", "0").
		Style("display", "flex").
		Style("padding", "12px").
		Style("box-sizing", "border-box").
		Style("visibility", visibility).
		Body(app.If(r.opened[m.mode], func() app.UI { return r.modeContent(m) }))
}

// modeContent is the component of a mode: its selection page or the editor of
// the entity selected.
func (r *Root) modeContent(m *modeView) app.UI {
	if m.picker != nil {
		// Both screens stay mounted — the page keeps its list, the editor its
		// unsaved edits and its iframes — and the hidden one is only invisible.
		return app.Div().
			Style("position", "relative").
			Style("display", "flex").
			Style("flex", "1").
			Style("min-width", "0").
			Style("min-height", "0").
			Body(
				screen(r.showList[m.mode], m.picker),
				screen(!r.showList[m.mode], app.Div().
					Style("display", "flex").
					Style("flex-direction", "column").
					Style("flex", "1").
					Style("min-height", "0").
					Body(
						app.Div().
							Style("display", "flex").
							Style("flex", "1").
							Style("min-height", "0").
							Body(m.ui),
					)),
			)
	}
	return m.ui
}

// screen is one screen of a mode with a selection page, shown or hidden: it
// fills the mode's slot, so the hidden one keeps its layout and its iframes.
// A shown screen inherits the visibility of its mode's slot — explicit
// "visible" would also show it on top of another mode, which Root hides with
// visibility — and a hidden one is hidden within the mode shown.
func screen(visible bool, body app.UI) app.UI {
	visibility := "hidden"
	if visible {
		visibility = "inherit"
	}
	return app.Div().
		Style("position", "absolute").
		Style("inset", "0").
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("visibility", visibility).
		Body(body)
}

// modeView is a mode and its component: the selection page of its entities
// (Library, Project), absent for Operation and History, which only show a
// Project.
type modeView struct {
	mode   Mode
	ui     app.Composer
	picker listPage
}

// listPage is the selection page of a mode with one: picker.Picker, behind an
// interface only for what Root does with it — ask it to reread the list when
// the page is shown, and mark the entity the main menu opened — so a test
// reaches the decision without the page being mounted.
type listPage interface {
	app.UI
	Reload(app.Context)
	Select(app.Context, uuid.UUID)
}

// renderThemeToggle switches the theme: auto, light, dark, then auto.
func (r *Root) renderThemeToggle() app.UI {
	var icon, title string
	switch r.themeMode {
	case "light":
		icon, title = "☀", "Light theme — click for dark"
	case "dark":
		icon, title = "☾", "Dark theme — click for auto"
	default:
		icon, title = "◑", "Auto theme — click for light"
	}

	return app.Button().
		Title(title).
		Style("margin", "0 10px").
		Style("padding", "4px 7px").
		Style("font-size", "15px").
		Style("line-height", "1").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "5px").
		Style("background", "var(--bg-elevated)").
		Style("color", "var(--text-2)").
		Text(icon).
		OnClick(func(ctx app.Context, e app.Event) {
			next := map[string]string{"auto": "light", "light": "dark", "dark": "auto"}
			r.setTheme(ctx, next[r.themeMode])
		})
}

// SetMode shows mode, opening it if it was not yet. The Library and the
// Project mode open the screen the person left there — the selection page or
// the editor of the entity last shown — and its list is reread when it is a
// page (task 73, task 85_1). The Bar follows the project of a mode that shows
// one, from the choice of it in the main menu on (selectProject); Library
// leaves it as it is.
func (r *Root) SetMode(mode Mode) {
	r.currentMode = mode
	first := !r.opened[mode]
	r.opened[mode] = true
	if m := r.modeView(mode); m != nil && m.picker != nil && first {
		// The screen is decided once: what the person left it at stays
		// through every return to the tab (task 85_1). A mode with nothing
		// selected shows its page; a page reads its list when it is shown
		// (task 73).
		r.showList[mode] = r.showList[mode] || r.selected[mode] == uuid.Nil
		if r.showList[mode] {
			m.picker.Reload(r.compoCtx)
		}
	}
	if id, ok := r.barProject(mode); ok {
		r.bar.SetProject(id)
	}
}
