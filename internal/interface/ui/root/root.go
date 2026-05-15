package root

import (
	"github.com/kipitix/growscada/internal/interface/ui/history"
	"github.com/kipitix/growscada/internal/interface/ui/library"
	"github.com/kipitix/growscada/internal/interface/ui/operation"
	"github.com/kipitix/growscada/internal/interface/ui/project"
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
	apiServerURL string
}

func NewRoot(anAPIServerURL string) *Root {
	return &Root{
		currentMode:  ModeLibrary,
		apiServerURL: anAPIServerURL,
	}
}

func (r *Root) OnMount(ctx app.Context) {
	ctx.Page().SetTitle("GrowSCADA")
}

func (r *Root) Render() app.UI {
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("height", "100vh").
		Style("font-family", "sans-serif").
		Body(
			app.Div().
				Attr("role", "tablist").
				Style("display", "flex").
				Style("flex-direction", "row").
				Style("border-bottom", "2px solid #ddd").
				Style("background", "#f8f8f8").
				Body(
					r.tab("Library", ModeLibrary),
					r.tab("Project", ModeProject),
					r.tab("Operation", ModeOperation),
					r.tab("History", ModeHistory),
				),
			app.Div().
				Style("flex", "1").
				Style("min-height", "0").
				Style("display", "flex").
				Style("padding", "12px").
				Style("box-sizing", "border-box").
				Body(
					app.If(r.currentMode == ModeLibrary, func() app.UI {
						return library.NewLibrary(r.apiServerURL)
					}).ElseIf(r.currentMode == ModeProject, func() app.UI {
						return &project.Project{}
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
		})

	if active {
		return tab.
			Style("border-bottom-color", "#0066cc").
			Style("color", "#0066cc").
			Style("font-weight", "600")
	}
	return tab.
		Style("color", "#555")
}

func (r *Root) SetMode(mode Mode) {
	r.currentMode = mode
}
