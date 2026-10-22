package uiutil

import (
	"html"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// WidgetIFrame is the sandboxed iframe of a Widget placed on a Scene, filling
// its container (see sceneLayerIFrame). The caller sets its srcdoc.
func WidgetIFrame() app.HTMLIFrame {
	return sceneLayerIFrame().
		Attr("sandbox", "allow-scripts").
		Style("display", "block")
}

// sceneLayerIFrame is the base of the iframes drawn on a Scene (a Widget, the
// Scene's background): filling its container, transparent and borderless.
// pointer-events:none lets mouse events reach whatever lies over or under it
// (Project's canvas and drag overlay). It sets no display and no sandbox.
//
// Its color-scheme is pinned to light, the default of the iframe's document:
// a browser paints an opaque backdrop under an iframe whose document's colour
// scheme differs from the iframe's (inherited from the dark theme), which
// would hide what lies under a transparent widget or a background that does
// not cover the Scene. It also keeps the document's defaults (text colour,
// form controls) independent of the UI theme.
func sceneLayerIFrame() app.HTMLIFrame {
	return app.IFrame().
		Attr("allowtransparency", "true").
		Style("color-scheme", "light").
		Style("width", "100%").
		Style("height", "100%").
		Style("border", "none").
		Style("background", "transparent").
		Style("pointer-events", "none")
}

// WidgetNamePlaceholder is the HTML shown in place of a Widget whose
// WidgetType is not loaded yet or deleted: the Widget's name, centred.
func WidgetNamePlaceholder(name, textColor string) string {
	return `<div style="display:flex;align-items:center;justify-content:center;height:100%;margin:0;font:11px sans-serif;color:` +
		html.EscapeString(textColor) + `">` + html.EscapeString(name) + `</div>`
}
