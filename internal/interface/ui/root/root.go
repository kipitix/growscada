package root

import (
	"github.com/kipitix/growscada/internal/interface/ui/history"
	"github.com/kipitix/growscada/internal/interface/ui/library"
	"github.com/kipitix/growscada/internal/interface/ui/operation"
	"github.com/kipitix/growscada/internal/interface/ui/project"
	"github.com/kipitix/growscada/internal/interface/ui/toast"
	"github.com/maxence-charriere/go-app/v10/pkg/app"
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
	currentMode  Mode
	themeMode    string // "auto" | "light" | "dark"
	apiServerURL string
}

func NewRoot(anAPIServerURL string) *Root {
	return &Root{
		currentMode: ModeLibrary,
		themeMode:   "auto",
		apiServerURL: anAPIServerURL,
	}
}

func (r *Root) OnMount(ctx app.Context) {
	ctx.Page().SetTitle("GrowSCADA")

	injectToastCSS()
	injectDesignCSS()

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
		if Mode(savedMode) != ModeUnknown {
			r.currentMode = Mode(savedMode)
		}
	})
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
		--accent: #0d9488;
		--accent-text: #ffffff;
		--accent-bg: #e6f7f5;
		--accent-border: #9fd9d0;
		--accent-soft: rgba(13,148,136,0.12);
		--accent-line: rgba(13,148,136,0.45);
		--ok: #1a7f37;
		--warm: #c2761a;
		--bg-app: #eceef1;
		--bg-panel: #ffffff;
		--bg-elev: #f6f7f9;
		--bg-code: #fbfbfd;
		--bg-input: #ffffff;
		--border-soft: #e8ebef;
		--border-strong: #c9d0d9;
		--text-dim: #5d6675;
		--text-faint: #97a0ad;
		--hover: rgba(20,30,50,0.04);
		--shadow: 0 1px 2px rgba(20,30,50,0.08);
		--grid: rgba(20,30,50,0.05);
		--error: #cc0000;
		--error-bg: #fff5f5;
		--error-border: #e0b0b0;
		--input-bg: #ffffff;
		--widget-bg: rgba(240,240,240,0.85);
		--widget-sel-bg: rgba(235,245,255,0.92);
		--dot-color: #cccccc;
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
		--accent: #15b8a6;
		--accent-text: #ffffff;
		--accent-bg: rgba(21,184,166,0.16);
		--accent-border: rgba(21,184,166,0.5);
		--accent-soft: rgba(21,184,166,0.16);
		--accent-line: rgba(21,184,166,0.5);
		--ok: #2ecc71;
		--warm: #e0922e;
		--bg-app: #161b22;
		--bg-panel: #1b212b;
		--bg-elev: #212934;
		--bg-code: #12161d;
		--bg-input: #0f141a;
		--border-soft: #232b35;
		--border-strong: #3a4452;
		--text-dim: #8a94a3;
		--text-faint: #5c6675;
		--hover: rgba(255,255,255,0.045);
		--shadow: 0 1px 3px rgba(0,0,0,0.4);
		--grid: rgba(255,255,255,0.04);
		--error: #ff6868;
		--error-bg: #2a1818;
		--error-border: #7a3838;
		--input-bg: #262626;
		--widget-bg: rgba(36,36,36,0.92);
		--widget-sel-bg: rgba(18,38,64,0.95);
		--dot-color: #282828;
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
		Style("font-family", "-apple-system, BlinkMacSystemFont, \"Segoe UI\", system-ui, sans-serif").
		Style("background", "var(--bg-app)").
		Style("color", "var(--text)").
		Body(
			&toast.Container{},
			app.Div().
				Attr("role", "tablist").
				Class("ed-top").
				Body(
					r.tab("Library", ModeLibrary),
					r.tab("Project", ModeProject),
					r.tab("Operation", ModeOperation),
					r.tab("History", ModeHistory),
					app.Div().Class("ed-spacer"),
					app.Div().Class("ed-brand").Body(
						app.Div().Class("ed-brand-mark"),
						app.Div().Class("ed-brand-name").Body(
							app.Text("Grow"),
							app.Span().Text("SCADA"),
						),
					),
					r.renderThemeToggle(),
				),
			app.Div().
				Style("flex", "1").
				Style("min-height", "0").
				Style("display", "flex").
				Style("box-sizing", "border-box").
				Style("background", "var(--bg-app)").
				Body(
					app.If(r.currentMode == ModeLibrary, func() app.UI {
						return library.NewLibrary(r.apiServerURL)
					}).ElseIf(r.currentMode == ModeProject, func() app.UI {
						return project.NewProject(r.apiServerURL)
					}).ElseIf(r.currentMode == ModeOperation, func() app.UI {
						return &operation.Operation{}
					}).Else(func() app.UI {
						return &history.History{}
					}),
				),
		)
}

func (r *Root) tab(label string, mode Mode) app.UI {
	active := r.currentMode == mode

	ariaSelected := "false"
	tabIdx := -1
	if active {
		ariaSelected = "true"
		tabIdx = 0
	}

	class := "ed-tab"
	if active {
		class += " active"
	}

	return app.Div().
		Attr("role", "tab").
		Attr("aria-selected", ariaSelected).
		Class(class).
		TabIndex(tabIdx).
		Text(label).
		OnClick(func(ctx app.Context, e app.Event) {
			r.currentMode = mode
			ctx.LocalStorage().Set("root:mode", string(mode))
		})
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
		Class("icon-btn").
		Title(title).
		Text(icon).
		OnClick(func(ctx app.Context, e app.Event) {
			next := map[string]string{"auto": "light", "light": "dark", "dark": "auto"}
			r.setTheme(ctx, next[r.themeMode])
		})
}

func (r *Root) SetMode(mode Mode) {
	r.currentMode = mode
}
