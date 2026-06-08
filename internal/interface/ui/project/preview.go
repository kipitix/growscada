package project

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/interface/ui/uidto"
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
		Style("width", "100%").
		Style("height", "100%").
		Style("border", "none").
		Style("pointer-events", "none").
		Style("display", "block")
}

func (p *widgetPreviewFrame) setSrcdoc() {
	elem := app.Window().Get("document").Call("getElementById", p.ID)
	if !elem.IsNull() && !elem.IsUndefined() {
		elem.Set("srcdoc", p.Srcdoc)
	}
}

func (p *widgetPreviewFrame) OnMount(ctx app.Context)  { p.setSrcdoc() }
func (p *widgetPreviewFrame) OnUpdate(ctx app.Context) { p.setSrcdoc() }

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
		Style("width", fmt.Sprintf("%dpx", t.NativeW)).
		Style("height", fmt.Sprintf("%dpx", t.NativeH)).
		Style("border", "none").
		Style("pointer-events", "none").
		Style("transform", fmt.Sprintf("scale(%.4f)", t.Scale)).
		Style("transform-origin", "0 0").
		Style("display", "block")
}

func (t *widgetThumbnailFrame) setSrcdoc() {
	elem := app.Window().Get("document").Call("getElementById", t.ID)
	if !elem.IsNull() && !elem.IsUndefined() {
		elem.Set("srcdoc", t.Srcdoc)
	}
}

func (t *widgetThumbnailFrame) OnMount(ctx app.Context)  { t.setSrcdoc() }
func (t *widgetThumbnailFrame) OnUpdate(ctx app.Context) { t.setSrcdoc() }

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

// buildSrcdoc constructs the iframe srcdoc for sandboxed widget preview.
// It builds an `inputs` JS object from inputValues keyed by port name, then
// calls render(inputs) if any ports are defined.
func buildSrcdoc(htmlTemplate, script string, inputValues map[string]string, ports []uidto.InputPortDTO) string {
	callRender := ""
	if len(ports) > 0 {
		parts := make([]string, 0, len(ports))
		for _, p := range ports {
			parts = append(parts, p.Name+":"+portValueToJS(inputValues[p.Name], p.TypeHint))
		}
		callRender = fmt.Sprintf("\nvar inputs={%s};\ntry{render(inputs);}catch(e){}", strings.Join(parts, ","))
	}
	// Escape </script> so a literal occurrence in user-authored JS cannot
	// terminate the enclosing <script> element and inject new HTML.
	safeScript := strings.ReplaceAll(script, "</script>", `<\/script>`)
	return fmt.Sprintf(
		`<!DOCTYPE html><html><head><meta http-equiv="Content-Security-Policy" content="connect-src 'none'"></head><body>%s<script>%s%s</script></body></html>`,
		htmlTemplate, safeScript, callRender)
}

// portValueToJS converts a user-entered string to a JS literal based on type hint.
// Values are validated/encoded to prevent JS injection in the preview srcdoc.
func portValueToJS(val, typeHint string) string {
	if val == "" {
		switch typeHint {
		case "integer":
			return "0"
		case "boolean":
			return "false"
		case "string":
			return `""`
		default:
			return "undefined"
		}
	}
	switch typeHint {
	case "integer":
		if _, err := strconv.ParseInt(val, 10, 64); err == nil {
			return val
		}
		return "0"
	case "boolean":
		if val == "true" || val == "false" {
			return val
		}
		return "false"
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}
