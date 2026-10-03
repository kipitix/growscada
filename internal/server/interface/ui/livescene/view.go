package livescene

import (
	"fmt"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// View renders a Scene 1:1 with its background and Widgets fed by live Tag
// values. It is purely presentational: whoever owns it supplies the state.
//
// go-app re-renders the View when any exported field changes, so its owner
// should pass in Tags only those the Widgets read (SceneTags): a change of
// any other Tag then costs the View nothing.
type View struct {
	app.Compo
	Scene       Scene
	Widgets     []Widget
	WidgetTypes map[string]WidgetType // by ID
	Tags        map[string]Tag        // by ID
	// Connected is false while the connection to the server is lost: a banner
	// is shown and every Widget is marked at least Uncertain.
	Connected bool
	// Theme is the UI theme ("light", "dark", "auto"). The colour of the
	// placeholder shown for a Widget without its WidgetType is read from the
	// page's CSS, so the View must re-render when it changes; the value itself
	// is not used. Widgets themselves are transparent, theme-independent.
	Theme string

	// srcdocs caches the live document of each WidgetType (by ID): it depends
	// on the WidgetType only, not on Tag values.
	srcdocs map[string]cachedSrcdoc
}

// cachedSrcdoc is a live document with what it was built from.
type cachedSrcdoc struct {
	htmlTemplate, script string
	srcdoc               string
}

func (v *View) Render() app.UI {
	textMuted := uiutil.IframeTextMuted()

	// Rebuilt on every render from the WidgetTypes in use, so the entries of
	// deleted or unused types are dropped.
	cached := v.srcdocs
	v.srcdocs = make(map[string]cachedSrcdoc, len(cached))
	for _, w := range v.Widgets {
		wt, ok := v.WidgetTypes[w.TypeID]
		if !ok {
			continue
		}
		if _, done := v.srcdocs[wt.ID]; done {
			continue
		}
		c, ok := cached[wt.ID]
		if !ok || c.htmlTemplate != wt.HtmlTemplate || c.script != wt.Script {
			c = cachedSrcdoc{wt.HtmlTemplate, wt.Script, uiutil.BuildLiveSrcdoc(wt.HtmlTemplate, wt.Script)}
		}
		v.srcdocs[wt.ID] = c
	}

	layers := make([]app.UI, 0, len(v.Widgets)+1)
	if v.Scene.BackgroundHTML != "" {
		layers = append(layers, v.renderBackground())
	}
	for _, w := range v.Widgets {
		layers = append(layers, v.renderWidget(w, textMuted))
	}

	canvas := app.Div().
		Style("position", "relative").
		Style("width", fmt.Sprintf("%dpx", v.Scene.Width)).
		Style("height", fmt.Sprintf("%dpx", v.Scene.Height)).
		Style("background-color", "var(--surface)").
		Style("flex-shrink", "0").
		// Centred while the Scene fits; once it does not, auto margins
		// collapse to 0 and it scrolls from its top-left corner unclipped.
		Style("margin", "auto").
		Style("overflow", "hidden").
		Body(layers...)

	return app.Div().
		Style("flex", "1").
		Style("min-height", "0").
		Style("display", "flex").
		Style("flex-direction", "column").
		Body(
			app.If(!v.Connected, func() app.UI {
				return app.Div().
					Attr("role", "alert").
					Style("flex-shrink", "0").
					Style("padding", "6px 12px").
					Style("background", "#c0392b").
					Style("color", "#fff").
					Style("font-size", "13px").
					Style("font-weight", "600").
					Text("No connection to the server — values may be out of date")
			}),
			// No padding: it would add scrollbars to a Scene that fits exactly.
			app.Div().
				Style("flex", "1").
				Style("min-height", "0").
				Style("display", "flex").
				Style("overflow", "auto").
				Style("background", "var(--bg-inset)").
				Body(canvas),
		)
}

// renderBackground renders the Scene's static background HTML under the
// Widgets, in an iframe sandboxed without scripts.
func (v *View) renderBackground() app.UI {
	return app.IFrame().
		Attr("sandbox", "").
		Attr("srcdoc", `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0;overflow:hidden}</style></head><body>`+v.Scene.BackgroundHTML+`</body></html>`).
		Style("position", "absolute").
		Style("inset", "0").
		Style("width", "100%").
		Style("height", "100%").
		Style("border", "none").
		Style("pointer-events", "none").
		Style("display", "block")
}

func (v *View) renderWidget(w Widget, textMuted string) app.UI {
	var srcdoc, inputsJSON string
	quality := QualityBad
	if wt, ok := v.WidgetTypes[w.TypeID]; ok {
		srcdoc = v.srcdocs[wt.ID].srcdoc
		inputsJSON = uiutil.InputsJSON(WidgetInputs(w, wt, v.Tags))
		quality = WidgetQuality(w, wt, v.Tags, v.Connected)
	} else {
		// WidgetType not loaded yet or deleted — show the widget name instead.
		srcdoc = uiutil.BuildLiveSrcdoc(uiutil.WidgetNamePlaceholder(w.Name, textMuted), "")
		inputsJSON = uiutil.InputsJSON(nil)
	}

	return app.Div().
		Style("position", "absolute").
		Style("left", "0").
		Style("top", "0").
		Style("width", fmt.Sprintf("%dpx", w.Size.Width)).
		Style("height", fmt.Sprintf("%dpx", w.Size.Height)).
		Style("transform-origin", "0 0").
		Style("transform", w.TransformMatrix.CSS).
		Style("z-index", fmt.Sprint(w.Position.Z)).
		Body(
			&widgetFrame{Srcdoc: srcdoc, InputsJSON: inputsJSON},
			renderQualityMarker(quality),
		)
}

// renderQualityMarker overlays the Widget with its Quality: a yellow frame for
// Uncertain, a red frame and dimming for Bad, nothing for Good.
func renderQualityMarker(q Quality) app.UI {
	marker := app.Div().
		Attr("data-quality", string(q)).
		Style("position", "absolute").
		Style("inset", "0").
		Style("box-sizing", "border-box").
		Style("pointer-events", "none")
	switch q {
	case QualityUncertain:
		return marker.Style("border", "3px solid #f1c40f")
	case QualityBad:
		return marker.
			Style("border", "3px solid #e74c3c").
			Style("background", "rgba(0,0,0,0.45)")
	default:
		return marker
	}
}
