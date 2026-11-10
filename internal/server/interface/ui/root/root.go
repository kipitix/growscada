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
	"github.com/kipitix/growscada/internal/server/interface/ui/library"
	"github.com/kipitix/growscada/internal/server/interface/ui/operation"
	"github.com/kipitix/growscada/internal/server/interface/ui/picker"
	"github.com/kipitix/growscada/internal/server/interface/ui/project"
	"github.com/kipitix/growscada/internal/server/interface/ui/projects"
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
)

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
	// selected holds the entity each mode shows, once its switcher or
	// selection page announced it: the Project of the modes with a project
	// switcher, and the Project or Library of the Project and Library modes.
	selected map[Mode]uuid.UUID
	// entityName is the name of the entity selected, as last read by the
	// selection page: the slim "← List · <name>" row of the editor.
	entityName map[Mode]string
	// showList is true while a mode shows its selection page (task 73)
	// instead of the editor of the entity selected. The tab opens the last
	// entity; with none it opens the page.
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
}

func NewRoot(anAPIServerURL string) *Root {
	r := &Root{
		currentMode: ModeLibrary,
		themeMode:   "auto",
	}
	r.client = apiclient.New(anAPIServerURL, http.DefaultClient, apiclient.OnServerVersion(r.observeServerVersion))
	r.project = project.NewProject(r.client)
	r.library = library.NewLibrary(r.client)
	r.operation = operation.NewOperation(r.client)
	r.bar = eventlog.NewBar(anAPIServerURL)
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
		{mode: ModeOperation, ui: r.operation, switcher: r.newSelector(ModeOperation)},
		{mode: ModeHistory, ui: &history.History{}, switcher: r.newSelector(ModeHistory)},
	}
	r.opened = map[Mode]bool{}
	r.selected = map[Mode]uuid.UUID{}
	r.entityName = map[Mode]string{}
	r.showList = map[Mode]bool{}
	return r
}

func (r *Root) OnMount(ctx app.Context) {
	r.compoCtx, r.mounted = ctx, true
	ctx.Page().SetTitle("GrowSCADA")

	// The tab opens the entity last shown: it is read here, before any mode
	// mounts, so the editor does not wait for its selection page (task 73).
	r.selected[ModeProject] = uiutil.LoadID(ctx, picker.ProjectKey)
	r.selected[ModeLibrary] = uiutil.LoadID(ctx, picker.LibraryKey)

	// A read of the entity the editor shows found it gone: the mode leaves it
	// at once, so the content the server removes after it is not loaded for
	// it, and shows its selection page with its list reread (task 73,
	// task 76). The Project says so through the content of its project, the
	// Library through a read of its Draft, its stack, its Commits or its
	// releases.
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
		// Only now, so a mode is not opened (and loaded) just for being
		// the default before the saved one is known.
		r.SetMode(mode)
	})
}

// newSelector is the project switcher of a mode that only selects projects
// (Operation, History): Project and Library list their entities on a
// selection page of their own (task 73).
func (r *Root) newSelector(mode Mode) *projects.Switcher {
	return projects.NewSelector(r.client, string(mode)+":projectID", func(id uuid.UUID) {
		r.selectProject(mode, id)
	})
}

// selectProject shows the project id (uuid.Nil: none) in mode, one of those
// whose compact switcher selects a project (Operation, History); it calls it
// on the UI goroutine. Operation reads nothing of the Bar: its own Player
// follows the Journal (ADR 0020); History reads it. If the Bar follows the
// project already, it resyncs now; else, if mode is shown, the Bar follows
// it, its stream resyncing as it opens; else mode reads it once shown
// (SetMode). The Project mode has a selection page instead, which announces
// through selectEntity (task 73).
func (r *Root) selectProject(mode Mode, id uuid.UUID) {
	r.selected[mode] = id
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
	r.selectEntity(mode, id, r.entityName[mode])
}

// showListPage shows the selection page of mode, rereading its list, and
// keeps the entity selected announced to the event stream as it is.
func (r *Root) showListPage(mode Mode) {
	r.showList[mode] = true
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
// there (task 76). The id must still be the one shown: a delayed 404 of an
// entity left meanwhile is not this one's.
func (r *Root) entityGone(mode Mode, id uuid.UUID) {
	if id == uuid.Nil || r.selected[mode] != id {
		return
	}
	r.selectEntity(mode, uuid.Nil, "")
	r.showListPage(mode)
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
// none for Library, which does not depend on one.
func (r *Root) barProject(mode Mode) (uuid.UUID, bool) {
	if mode == ModeLibrary {
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
			app.Div().
				Attr("role", "tablist").
				Style("display", "flex").
				Style("flex-direction", "row").
				Style("align-items", "center").
				Style("border-bottom", "2px solid var(--border)").
				Style("background", "var(--bg-elevated)").
				Body(
					r.tab("Library", ModeLibrary),
					r.tab("Project", ModeProject),
					r.tab("Operation", ModeOperation),
					r.tab("History", ModeHistory),
					app.Div().Style("flex", "1"),
					r.renderThemeToggle(),
				),
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
// the entity selected, under its project switcher if it has one.
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
						r.renderBackBar(m.mode),
						app.Div().
							Style("display", "flex").
							Style("flex", "1").
							Style("min-height", "0").
							Body(m.ui),
					)),
			)
	}
	if m.switcher == nil {
		return m.ui
	}
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Body(
			m.switcher,
			app.Div().
				Style("display", "flex").
				Style("flex", "1").
				Style("min-height", "0").
				Body(m.ui),
		)
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

// renderBackBar is the slim "← List · <name>" row at the top left of the
// editor of the Library and Project modes (task 73): it shows the selection
// page and keeps the entity announced to the event stream. An entity a read
// finds gone returns to that page too, without the user asking (task 76).
func (r *Root) renderBackBar(mode Mode) app.UI {
	label := "← List"
	if name := r.entityName[mode]; name != "" {
		label += " · " + name
	}
	return app.Div().
		Attr("data-back-to-list", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("height", "26px").
		Style("flex-shrink", "0").
		Style("padding", "0 10px").
		Style("box-sizing", "border-box").
		Style("border-bottom", "1px solid var(--border)").
		Style("background", "var(--bg-elevated)").
		Style("font-size", "12px").
		Style("color", "var(--text-2)").
		Style("cursor", "pointer").
		Style("user-select", "none").
		Text(label).
		OnClick(func(ctx app.Context, e app.Event) {
			r.showListPage(mode)
		})
}

// modeView is a mode and its component: the switcher of the project it shows
// (Operation, History) or the selection page of its entities (Library,
// Project). At most one of them is set.
type modeView struct {
	mode     Mode
	ui       app.Composer
	switcher *projects.Switcher
	picker   *picker.Picker
}

func (r *Root) tab(label string, mode Mode) app.UI {
	active := r.currentMode == mode

	ariaSelected := "false"
	tabIdx := -1
	if active {
		ariaSelected = "true"
		tabIdx = 0
	}

	tab := app.Div().
		Attr("role", "tab").
		Attr("aria-selected", ariaSelected).
		TabIndex(tabIdx).
		Style("padding", "10px 20px").
		Style("cursor", "pointer").
		Style("font-size", "14px").
		Style("user-select", "none").
		Style("border-bottom", "2px solid transparent").
		Style("margin-bottom", "-2px").
		Text(label).
		OnClick(func(ctx app.Context, e app.Event) {
			r.SetMode(mode)
			ctx.LocalStorage().Set("root:mode", string(mode))
		})

	if active {
		return tab.
			Style("border-bottom-color", "var(--accent)").
			Style("color", "var(--accent)").
			Style("font-weight", "600")
	}
	return tab.
		Style("color", "var(--text-2)")
}

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

// SetMode shows mode, opening it if it was not yet. A tab of the Library or
// the Project mode opens the entity last shown, or its selection page when
// there is none (task 73). The Bar follows the project of a mode that shows
// one, from its switcher's announcement on (selectProject); Library leaves it
// as it is.
func (r *Root) SetMode(mode Mode) {
	r.currentMode = mode
	r.opened[mode] = true
	if m := r.modeView(mode); m != nil && m.picker != nil {
		r.showList[mode] = r.selected[mode] == uuid.Nil
		if r.showList[mode] {
			// The page reads its list whenever it is shown (task 73).
			m.picker.Reload(r.compoCtx)
		}
	}
	if id, ok := r.barProject(mode); ok {
		r.bar.SetProject(id)
	}
}
