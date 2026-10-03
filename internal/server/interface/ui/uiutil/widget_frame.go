package uiutil

import (
	"html"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// WidgetIFrame is the sandboxed iframe of a Widget placed on a Scene, filling
// its container. pointer-events:none lets mouse events reach whatever lies
// over or under it (Project's drag overlay). The caller sets its srcdoc.
//
// Its color-scheme is pinned to light, the default of the widget's document:
// a browser paints an opaque backdrop under an iframe whose document's colour
// scheme differs from the iframe's (inherited from the dark theme), which
// would hide the Scene under a transparent widget. It also keeps the widget's
// defaults (text colour, form controls) independent of the UI theme.
func WidgetIFrame() app.HTMLIFrame {
	return app.IFrame().
		Attr("sandbox", "allow-scripts").
		Attr("allowtransparency", "true").
		Style("color-scheme", "light").
		Style("width", "100%").
		Style("height", "100%").
		Style("border", "none").
		Style("background", "transparent").
		Style("pointer-events", "none").
		Style("display", "block")
}

// WidgetNamePlaceholder is the HTML shown in place of a Widget whose
// WidgetType is not loaded yet or deleted: the Widget's name, centred.
func WidgetNamePlaceholder(name, textColor string) string {
	return `<div style="display:flex;align-items:center;justify-content:center;height:100%;margin:0;font:11px sans-serif;color:` +
		html.EscapeString(textColor) + `">` + html.EscapeString(name) + `</div>`
}
