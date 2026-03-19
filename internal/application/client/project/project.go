package project

import "github.com/maxence-charriere/go-app/v10/pkg/app"

type Project struct {
	app.Compo
}

func (p *Project) Render() app.UI {
	return app.Div().Body(
		app.H1().Text("Project"),
	)
}