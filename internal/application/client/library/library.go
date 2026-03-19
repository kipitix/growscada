package library

import "github.com/maxence-charriere/go-app/v10/pkg/app"

type Library struct {
	app.Compo
}

func (l *Library) Render() app.UI {
	return app.Div().Body(
		app.H1().Text("Library"),
	)
}
