package project

import (
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// Not every button of the mode goes through uiutil.Button (task 80): the
// dialogs keep their footer actions — the 4px 12px/13px metric and the solid
// accent of the primary one —, and the reset icon of a property field, the bare
// inline text actions and the file picker's label each have a layout the
// presets do not describe. Everything else uses the presets, so no second copy
// of the palette or of the disabled state remains.

// rowButton is one action of a row of a list — a Release's Deploy, a Commit's
// download, the author's change. kind "accent", "error" or "" picks its
// colours, title is its hover text. options name the row (app.EventScope), so a
// reused element does not keep the handler of the row it showed before.
func rowButton(label, kind string, enabled bool, title string, onClick func(app.Context), options ...app.EventOption) app.HTMLButton {
	return uiutil.Button(label, kind, enabled, title, uiutil.ButtonRow,
		func(ctx app.Context, _ app.Event) { onClick(ctx) }, options...)
}
