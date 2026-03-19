package operation

import "github.com/maxence-charriere/go-app/v10/pkg/app"

type Operation struct {
	app.Compo
}

func (o *Operation) Render() app.UI {
	return app.Div().Body(
		app.H1().Text("Operation"),
	)
}