package project

import (
	"fmt"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── Left panel: Widget Types ───────────────────────────────────────────────────

func (p *Project) renderWidgetTypePanel() app.UI {
	items := make([]app.UI, len(p.widgetTypes))
	for i, wt := range p.widgetTypes {
		id := wt.ID
		name := wt.Name
		w, h := wt.DefaultWidth, wt.DefaultHeight
		if w <= 0 {
			w = 100
		}
		if h <= 0 {
			h = 100
		}
		items[i] = app.Div().
			Style("padding", "7px 10px").
			Style("margin-bottom", "4px").
			Style("border", "1px solid var(--border-input)").
			Style("border-radius", "4px").
			Style("background", "var(--bg-hover)").
			Style("color", "var(--text)").
			Style("cursor", "grab").
			Style("user-select", "none").
			Body(
				app.Div().
					Style("font-size", "13px").
					Style("white-space", "nowrap").
					Style("overflow", "hidden").
					Style("text-overflow", "ellipsis").
					Text(name),
				app.Div().
					Style("font-size", "11px").
					Style("color", "var(--text-3)").
					Style("margin-top", "2px").
					Text(fmt.Sprintf("%d × %d", w, h)),
			).
			Draggable(true).
			OnDragStart(func(ctx app.Context, e app.Event) {
				p.draggedTypeID = id
				e.Get("dataTransfer").Call("setData", "text/plain", id)
				e.Get("dataTransfer").Set("effectAllowed", "copy")
			})
	}

	var listBody app.UI
	if len(items) == 0 {
		listBody = app.Div().
			Style("font-size", "13px").
			Style("color", "var(--text-muted)").
			Text("No widget types. Create them in the Library tab.")
	} else {
		listBody = app.Div().Body(items...)
	}

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("width", "180px").
		Style("flex-shrink", "0").
		Style("min-height", "0").
		Style("border-right", "1px solid var(--border)").
		Style("padding", "12px 8px").
		Body(
			app.H3().
				Style("margin", "0 0 10px 0").
				Style("font-size", "13px").
				Style("font-weight", "600").
				Style("color", "var(--text)").
				Style("text-transform", "uppercase").
				Style("letter-spacing", "0.5px").
				Text("Widget Types"),
			app.Div().
				Style("flex", "1").
				Style("overflow-y", "auto").
				Body(listBody),
		)
}
