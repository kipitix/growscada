package runtime

import "github.com/maxence-charriere/go-app/v10/pkg/app"

type Runtime struct {
	app.Compo
}

func (r *Runtime) Render() app.UI {
	return app.Div().Body(
		app.H1().Text("Runtime"),
	)
}
