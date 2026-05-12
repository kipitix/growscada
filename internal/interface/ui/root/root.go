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
		currentMode:  ModeLibrary, // Default mode
		apiServerURL: anAPIServerURL,
	}
}

func (r *Root) Render() app.UI {
	return app.Div().Body(
		// Toolbar with radio buttons
		app.Div().Class("toolbar").Body(
			app.Div().Class("mode-selector").Body(
				r.radioOption("Library", ModeLibrary),
				r.radioOption("Project", ModeProject),
				r.radioOption("Operation", ModeOperation),
				r.radioOption("History", ModeHistory),
			),
		),

		// Content
		app.Div().Class("content").Body(
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

func (r *Root) radioOption(label string, mode Mode) app.UI {
	return app.Label().Class("radio-option").Body(
		app.Input().
			Type("radio").
			Name("mode").
			Value(string(mode)).
			Checked(r.currentMode == mode).
			OnChange(func(ctx app.Context, e app.Event) {
				r.currentMode = mode
			}),
		app.Text(label),
	)
}

func (r *Root) SetMode(mode Mode) {
	r.currentMode = mode
}
