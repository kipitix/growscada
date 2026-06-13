package project

import (
	"fmt"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/interface/ui/uiutil"
)

// widgetPreviewFrame renders a sandboxed iframe that fills its container.
// Used on the scene canvas — the parent div sets the exact pixel size.
// pointer-events:none ensures mouse events pass through to the drag overlay.
type widgetPreviewFrame struct {
	app.Compo
	ID     string
	Srcdoc string
}

func (p *widgetPreviewFrame) Render() app.UI {
	return app.IFrame().
		ID(p.ID).
		Attr("sandbox", "allow-scripts").
		Attr("allowtransparency", "true").
		Style("width", "100%").
		Style("height", "100%").
		Style("border", "none").
		Style("background", "transparent").
		Style("pointer-events", "none").
		Style("display", "block")
}

func (p *widgetPreviewFrame) OnMount(ctx app.Context)  { uiutil.SetIframeSrcdoc(p.ID, p.Srcdoc) }
func (p *widgetPreviewFrame) OnUpdate(ctx app.Context) { uiutil.SetIframeSrcdoc(p.ID, p.Srcdoc) }

// widgetThumbnailFrame renders a sandboxed iframe at the widget's native size,
// then scales it down with CSS transform. The parent container must have
// overflow:hidden and be sized to NativeW*Scale × NativeH*Scale.
type widgetThumbnailFrame struct {
	app.Compo
	ID      string
	Srcdoc  string
	NativeW int
	NativeH int
	Scale   float64
}

func (t *widgetThumbnailFrame) Render() app.UI {
	return app.IFrame().
		ID(t.ID).
		Attr("sandbox", "allow-scripts").
		Attr("allowtransparency", "true").
		Style("width", fmt.Sprintf("%dpx", t.NativeW)).
		Style("height", fmt.Sprintf("%dpx", t.NativeH)).
		Style("border", "none").
		Style("background", "transparent").
		Style("pointer-events", "none").
		Style("transform", fmt.Sprintf("scale(%.4f)", t.Scale)).
		Style("transform-origin", "0 0").
		Style("display", "block")
}

func (t *widgetThumbnailFrame) OnMount(ctx app.Context)  { uiutil.SetIframeSrcdoc(t.ID, t.Srcdoc) }
func (t *widgetThumbnailFrame) OnUpdate(ctx app.Context) { uiutil.SetIframeSrcdoc(t.ID, t.Srcdoc) }

// simInputField is a controlled input that explicitly sets the DOM value
// property on mount and update, because go-app's virtual DOM removes the
// attribute for empty strings instead of setting value="", which leaves the
// input showing stale text when switching between widgets.
type simInputField struct {
	app.Compo
	FieldID  string
	Val      string
	PortName string
	OnChange func(string)
	lastVal  string
}

func (f *simInputField) Render() app.UI {
	onChange := f.OnChange
	portName := f.PortName
	return app.Input().
		ID(f.FieldID).
		Type("text").
		Value(f.Val).
		Style("width", "100%").
		Style("font-size", "12px").
		Style("padding", "3px 6px").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "3px").
		Style("background", "var(--input-bg)").
		Style("color", "var(--text)").
		Style("box-sizing", "border-box").
		OnInput(func(ctx app.Context, e app.Event) {
			if onChange != nil {
				onChange(ctx.JSSrc().Get("value").String())
			}
		}, app.EventScope(portName))
}

func (f *simInputField) setDOMValue(ctx app.Context) {
	fieldID := f.FieldID
	val := f.Val
	ctx.Defer(func(ctx app.Context) {
		elem := app.Window().Get("document").Call("getElementById", fieldID)
		if !elem.IsNull() && !elem.IsUndefined() {
			elem.Set("value", val)
		}
	})
}

func (f *simInputField) OnMount(ctx app.Context) {
	f.lastVal = f.Val
	f.setDOMValue(ctx)
}

func (f *simInputField) OnUpdate(ctx app.Context) {
	if f.Val == f.lastVal {
		return
	}
	f.lastVal = f.Val
	f.setDOMValue(ctx)
}

