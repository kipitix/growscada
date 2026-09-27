package history

import "github.com/maxence-charriere/go-app/v10/pkg/app"

type History struct {
	app.Compo
}

func (h *History) Render() app.UI {
	return app.Div().Body(
		app.H1().Text("History"),
	)
}
