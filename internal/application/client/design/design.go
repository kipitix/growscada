package design

import "github.com/maxence-charriere/go-app/v10/pkg/app"

type design struct {
	app.Compo
}

func (d *design) Render() app.UI {
	return app.Div().Body(
		app.H1().Text("Design"),
	)
}
