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
		if Mode(savedMode) != ModeUnknown {
			r.currentMode = Mode(savedMode)
		}
	})
}

func (r *Root) setTheme(ctx app.Context, mode string) {
	r.themeMode = mode
	ctx.LocalStorage().Set("root:theme", mode)
	injectThemeCSS(mode)
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
		Style("font-family", "sans-serif").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Body(
			&toast.Container{},
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
				Style("display", "flex").
				Style("padding", "12px").
				Style("box-sizing", "border-box").
				Style("background", "var(--bg)").
				Body(
					app.If(r.currentMode == ModeLibrary, func() app.UI {
						return library.NewLibrary(r.apiServerURL, r.themeMode)
					}).ElseIf(r.currentMode == ModeProject, func() app.UI {
						return project.NewProject(r.apiServerURL, r.themeMode)
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
			r.currentMode = mode
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

func (r *Root) SetMode(mode Mode) {
	r.currentMode = mode
}
