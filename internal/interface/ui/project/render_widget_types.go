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

		defaultW := wt.DefaultWidth
		defaultH := wt.DefaultHeight
		if defaultW <= 0 {
			defaultW = 120
		}
		if defaultH <= 0 {
			defaultH = 60
		}

		// Scale the thumbnail to fit inside the sidebar panel.
		// The iframe renders at the widget's native size and is shrunk via CSS
		// transform; the outer container is sized to the scaled dimensions.
		thumbW := p.widgetTypeWidth - 16
		if thumbW < 50 {
			thumbW = 50
		}
		scale := float64(thumbW) / float64(defaultW)
		thumbH := int(float64(defaultH) * scale)
		const maxThumbH = 120
		if thumbH > maxThumbH {
			thumbH = maxThumbH
			scale = float64(thumbH) / float64(defaultH)
			thumbW = int(float64(defaultW) * scale)
		}
		if thumbW < 50 {
			thumbW = 50
		}

		srcdoc := buildSrcdoc(wt.HtmlTemplate, wt.Script, nil, wt.InputPorts, iframeBgColor())

		var previewEl app.UI
		if wt.HtmlTemplate != "" {
			previewEl = app.Div().
				Style("width", fmt.Sprintf("%dpx", thumbW)).
				Style("height", fmt.Sprintf("%dpx", thumbH)).
				Style("overflow", "hidden").
				Style("border-radius", "3px").
				Style("flex-shrink", "0").
				Body(
					&widgetThumbnailFrame{
						ID:      "wt-thumb-" + id,
						Srcdoc:  srcdoc,
						NativeW: defaultW,
						NativeH: defaultH,
						Scale:   scale,
					},
				)
		} else {
			// No HTML defined yet: show a plain placeholder rectangle.
			previewEl = app.Div().
				Style("width", fmt.Sprintf("%dpx", thumbW)).
				Style("height", fmt.Sprintf("%dpx", thumbH)).
				Style("border", "1px dashed var(--border-input)").
				Style("border-radius", "3px").
				Style("display", "flex").
				Style("align-items", "center").
				Style("justify-content", "center").
				Style("flex-shrink", "0").
				Body(
					app.Span().
						Style("font-size", "10px").
						Style("color", "var(--text-muted)").
						Text("no HTML"),
				)
		}

		items[i] = app.Div().
			Style("padding", "6px").
			Style("margin-bottom", "6px").
			Style("border", "1px solid var(--border-input)").
			Style("border-radius", "4px").
			Style("background", "var(--bg-hover)").
			Style("cursor", "grab").
			Style("user-select", "none").
			Body(
				previewEl,
				app.Div().
					Style("font-size", "12px").
					Style("color", "var(--text)").
					Style("white-space", "nowrap").
					Style("overflow", "hidden").
					Style("text-overflow", "ellipsis").
					Style("margin-top", "4px").
					Text(name),
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
		Style("width", fmt.Sprintf("%dpx", p.widgetTypeWidth)).
		Style("flex-shrink", "0").
		Style("min-height", "0").
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
