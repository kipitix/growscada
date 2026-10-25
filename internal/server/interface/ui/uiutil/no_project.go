package uiutil

import "github.com/maxence-charriere/go-app/v11/pkg/app"

// NoProject is what a mode showing a project's content shows while no
// project is selected.
func NoProject() app.UI {
	return app.Div().
		Attr("data-no-project", "true").
		Style("flex", "1").
		Style("display", "flex").
		Style("align-items", "center").
		Style("justify-content", "center").
		Style("color", "var(--text-muted)").
		Style("font-size", "14px").
		Text("No project selected: create one or pick one above")
}
