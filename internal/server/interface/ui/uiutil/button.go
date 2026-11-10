package uiutil

import (
	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// ButtonLayout is the place a button sits in, which fixes its metrics: the
// rows of a page, a mode's slim header or a dialog (task 77).
type ButtonLayout string

const (
	// ButtonPage is an action of a page's row, e.g. the picker's
	// Open/Edit/Delete.
	ButtonPage ButtonLayout = "page"
	// ButtonBar is an action of a mode's header.
	ButtonBar ButtonLayout = "bar"
	// ButtonSmall is an action of a dialog: it shares the row's width.
	ButtonSmall ButtonLayout = "small"
)

// Button renders one action: kind "accent", "error" or "" picks its palette,
// layout its metrics, title its hover text, and a disabled one is dimmed and
// does nothing. options go to the click handler: a button of a list gives it
// the row's identity, so a reused element does not keep the handler of the row
// it showed before (app.EventScope).
//
// The palette and the disabled state of the picker's and the Library's buttons
// live here alone (task 77); the Project mode's builders still carry their own
// copies of both.
func Button(text, kind string, enabled bool, title string, layout ButtonLayout, onClick func(app.Context, app.Event), options ...app.EventOption) app.UI {
	border, bg, fg := buttonColors(kind)
	b := app.Button().
		Title(title).
		Style("border", "1px solid "+border).
		Style("border-radius", "4px").
		Style("background", bg).
		Style("color", fg).
		Text(text)
	dimmed := "0.5"
	switch layout {
	case ButtonBar:
		b = b.Style("padding", "2px 9px").Style("font-size", "12px")
		dimmed = "0.4"
	case ButtonSmall:
		b = b.Style("flex", "1").Style("padding", "4px 0").Style("font-size", "13px")
		dimmed = "0.4"
	default:
		b = b.Style("padding", "3px 10px").Style("font-size", "12px")
	}
	if !enabled {
		return b.Style("opacity", dimmed).Style("cursor", "default").Disabled(true)
	}
	return b.Style("cursor", "pointer").OnClick(onClick, options...)
}

// buttonColors is the palette of a button: its border, background and text.
// kind "accent", "error" or anything else for the neutral one.
func buttonColors(kind string) (border, bg, fg string) {
	border, bg, fg = "var(--border-input)", "var(--bg-hover)", "var(--text)"
	switch kind {
	case "accent":
		border, bg, fg = "var(--accent-border)", "var(--accent-bg)", "var(--accent)"
	case "error":
		border, bg, fg = "var(--error-border)", "var(--error-bg)", "var(--error)"
	}
	return border, bg, fg
}
