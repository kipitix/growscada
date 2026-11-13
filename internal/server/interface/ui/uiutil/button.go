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
	// ButtonSmall is an action of a dialog or of a table's filter row: it
	// shares the row's width.
	ButtonSmall ButtonLayout = "small"
	// ButtonRow is an action of a row of a list, e.g. a Release's Deploy or
	// a Commit's download.
	ButtonRow ButtonLayout = "row"
)

// Button renders one action: kind "accent", "error" or "" picks its palette,
// layout its metrics, title its hover text, and a disabled one is dimmed and
// does nothing. options go to the click handler: a button of a list gives it
// the row's identity, so a reused element does not keep the handler of the row
// it showed before (app.EventScope).
//
// The palette and the disabled state of every mode's buttons live here alone:
// the picker's page, the Library's and the Project's headers and dialogs, and
// the rows of their lists (task 77, task 80).
func Button(text, kind string, enabled bool, title string, layout ButtonLayout, onClick func(app.Context, app.Event), options ...app.EventOption) app.HTMLButton {
	return button(kind, enabled, title, layout, app.Text(text), onClick, options...)
}

// IconButton is Button with an icon and no text: the picker's actions
// (task 82). The icon is the whole face of the button, so the title is its
// accessible name — a hover text alone does not give one — and a destructive
// action keeps its confirmation.
func IconButton(icon app.UI, kind string, enabled bool, title string, layout ButtonLayout, onClick func(app.Context, app.Event), options ...app.EventOption) app.HTMLButton {
	return button(kind, enabled, title, layout, icon, onClick, options...).
		Attr("aria-label", title).
		Style("display", "inline-flex").
		Style("align-items", "center").
		Style("justify-content", "center").
		// An icon is the whole face: the size of the button comes from the
		// layout's CSS, never from the padding of a text button.
		Style("padding", "0")
}

// button renders one action around the content it shows.
func button(kind string, enabled bool, title string, layout ButtonLayout, content app.UI, onClick func(app.Context, app.Event), options ...app.EventOption) app.HTMLButton {
	border, bg, fg := buttonColors(kind)
	b := app.Button().
		Title(title).
		Style("border", "1px solid "+border).
		Style("border-radius", "4px").
		Style("background", bg).
		Style("color", fg).
		Body(content)
	dimmed := "0.5"
	switch layout {
	case ButtonBar:
		b = b.Style("padding", "2px 9px").Style("font-size", "12px")
		dimmed = "0.4"
	case ButtonSmall:
		b = b.Style("flex", "1").Style("padding", "4px 0").Style("font-size", "13px")
		dimmed = "0.4"
	case ButtonRow:
		b = b.Style("padding", "1px 7px").Style("font-size", "11px")
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
	case "ghost":
		// An action that stays out of the way until it is looked at, e.g. the
		// pencil of a field edited in place (task 82).
		border, bg, fg = "transparent", "transparent", "var(--text-muted)"
	}
	return border, bg, fg
}
